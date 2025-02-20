package activation

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/spacemeshos/post/shared"
	"github.com/spacemeshos/post/verifying"
	"golang.org/x/exp/maps"

	"github.com/spacemeshos/go-spacemesh/activation/wire"
	"github.com/spacemeshos/go-spacemesh/atxsdata"
	"github.com/spacemeshos/go-spacemesh/codec"
	"github.com/spacemeshos/go-spacemesh/common/types"
	"github.com/spacemeshos/go-spacemesh/datastore"
	"github.com/spacemeshos/go-spacemesh/events"
	"github.com/spacemeshos/go-spacemesh/log"
	mwire "github.com/spacemeshos/go-spacemesh/malfeasance/wire"
	"github.com/spacemeshos/go-spacemesh/p2p"
	"github.com/spacemeshos/go-spacemesh/p2p/pubsub"
	"github.com/spacemeshos/go-spacemesh/signing"
	"github.com/spacemeshos/go-spacemesh/sql"
	"github.com/spacemeshos/go-spacemesh/sql/atxs"
	"github.com/spacemeshos/go-spacemesh/sql/identities"
	"github.com/spacemeshos/go-spacemesh/system"
)

var (
	errKnownAtx      = errors.New("known atx")
	errMalformedData = fmt.Errorf("%w: malformed data", pubsub.ErrValidationReject)
	errWrongHash     = fmt.Errorf("%w: incorrect hash", pubsub.ErrValidationReject)
	errMaliciousATX  = errors.New("malicious atx")
)

// Handler processes the atxs received from all nodes and their validity status.
type Handler struct {
	local           p2p.Peer
	cdb             *datastore.CachedDB
	atxsdata        *atxsdata.Data
	edVerifier      *signing.EdVerifier
	clock           layerClock
	publisher       pubsub.Publisher
	tickSize        uint64
	goldenATXID     types.ATXID
	nipostValidator nipostValidator
	beacon          AtxReceiver
	tortoise        system.Tortoise
	log             log.Log
	fetcher         system.Fetcher

	signerMtx sync.Mutex
	signers   map[types.NodeID]*signing.EdSigner

	// inProgress map gathers ATXs that are currently being processed.
	// It's used to avoid processing the same ATX twice.
	inProgress   map[types.ATXID][]chan error
	inProgressMu sync.Mutex
}

// NewHandler returns a data handler for ATX.
func NewHandler(
	local p2p.Peer,
	cdb *datastore.CachedDB,
	atxsdata *atxsdata.Data,
	edVerifier *signing.EdVerifier,
	c layerClock,
	pub pubsub.Publisher,
	fetcher system.Fetcher,
	tickSize uint64,
	goldenATXID types.ATXID,
	nipostValidator nipostValidator,
	beacon AtxReceiver,
	tortoise system.Tortoise,
	log log.Log,
) *Handler {
	return &Handler{
		local:           local,
		cdb:             cdb,
		atxsdata:        atxsdata,
		edVerifier:      edVerifier,
		clock:           c,
		publisher:       pub,
		tickSize:        tickSize,
		goldenATXID:     goldenATXID,
		nipostValidator: nipostValidator,
		log:             log,
		fetcher:         fetcher,
		beacon:          beacon,
		tortoise:        tortoise,

		signers:    make(map[types.NodeID]*signing.EdSigner),
		inProgress: make(map[types.ATXID][]chan error),
	}
}

func (h *Handler) Register(sig *signing.EdSigner) {
	h.signerMtx.Lock()
	defer h.signerMtx.Unlock()
	if _, exists := h.signers[sig.NodeID()]; exists {
		h.log.With().Error("signing key already registered", log.ShortStringer("id", sig.NodeID()))
		return
	}

	h.log.With().Info("registered signing key", log.ShortStringer("id", sig.NodeID()))
	h.signers[sig.NodeID()] = sig
}

func (h *Handler) syntacticallyValidate(ctx context.Context, atx *wire.ActivationTxV1) error {
	if atx.NIPost == nil {
		return fmt.Errorf("nil nipost for atx %s", atx.ID())
	}
	current := h.clock.CurrentLayer().GetEpoch()
	if atx.PublishEpoch > current+1 {
		return fmt.Errorf("atx publish epoch is too far in the future: %d > %d", atx.PublishEpoch, current+1)
	}
	if atx.PositioningATXID == types.EmptyATXID {
		return errors.New("empty positioning atx")
	}

	switch {
	case atx.PrevATXID == types.EmptyATXID:
		if atx.InitialPost == nil {
			return errors.New("no prev atx declared, but initial post is not included")
		}
		if atx.NodeID == nil {
			return errors.New("no prev atx declared, but node id is missing")
		}
		if atx.VRFNonce == nil {
			return errors.New("no prev atx declared, but vrf nonce is missing")
		}
		if atx.CommitmentATXID == nil {
			return errors.New("no prev atx declared, but commitment atx is missing")
		}
		if *atx.CommitmentATXID == types.EmptyATXID {
			return errors.New("empty commitment atx")
		}
		if atx.Sequence != 0 {
			return errors.New("no prev atx declared, but sequence number not zero")
		}

		// Use the NIPost's Post metadata, while overriding the challenge to a zero challenge,
		// as expected from the initial Post.
		initialPostMetadata := types.PostMetadata{
			Challenge:     shared.ZeroChallenge,
			LabelsPerUnit: atx.NIPost.PostMetadata.LabelsPerUnit,
		}
		if err := h.nipostValidator.VRFNonce(
			atx.SmesherID, *atx.CommitmentATXID, *atx.VRFNonce, initialPostMetadata.LabelsPerUnit, atx.NumUnits,
		); err != nil {
			return fmt.Errorf("invalid vrf nonce: %w", err)
		}
		post := wire.PostFromWireV1(atx.InitialPost)
		if err := h.nipostValidator.Post(
			ctx, atx.SmesherID, *atx.CommitmentATXID, post, &initialPostMetadata, atx.NumUnits,
		); err != nil {
			return fmt.Errorf("invalid initial post: %w", err)
		}
	default:
		if atx.NodeID != nil {
			return errors.New("prev atx declared, but node id is included")
		}
		if atx.InitialPost != nil {
			return errors.New("prev atx declared, but initial post is included")
		}
		if atx.CommitmentATXID != nil {
			return errors.New("prev atx declared, but commitment atx is included")
		}
	}
	return nil
}

func (h *Handler) syntacticallyValidateDeps(
	ctx context.Context,
	atx *wire.ActivationTxV1,
) (leaves uint64, effectiveNumUnits uint32, proof *mwire.MalfeasanceProof, err error) {
	var commitmentATX types.ATXID

	if atx.PrevATXID == types.EmptyATXID {
		if err := h.nipostValidator.InitialNIPostChallengeV1(&atx.NIPostChallengeV1, h.cdb, h.goldenATXID); err != nil {
			return 0, 0, nil, err
		}
		effectiveNumUnits = atx.NumUnits
		commitmentATX = *atx.CommitmentATXID
	} else {
		commitmentATX, err = atxs.CommitmentATX(h.cdb, atx.SmesherID)
		if err != nil {
			return 0, 0, nil, fmt.Errorf("commitment atx for %s not found: %w", atx.SmesherID, err)
		}
		effectiveNumUnits, err = h.validateNonInitialAtx(ctx, atx, commitmentATX)
		if err != nil {
			return 0, 0, nil, err
		}
	}

	err = h.nipostValidator.PositioningAtx(atx.PositioningATXID, h.cdb, h.goldenATXID, atx.PublishEpoch)
	if err != nil {
		return 0, 0, nil, err
	}

	expectedChallengeHash := atx.NIPostChallengeV1.Hash()
	h.log.WithContext(ctx).
		With().
		Info("validating nipost", log.String("expected_challenge_hash", expectedChallengeHash.String()), atx.ID())

	leaves, err = h.nipostValidator.NIPost(
		ctx,
		atx.SmesherID,
		commitmentATX,
		wire.NiPostFromWireV1(atx.NIPost),
		expectedChallengeHash,
		atx.NumUnits,
		PostSubset([]byte(h.local)), // use the local peer ID as seed for random subset
	)
	var invalidIdx *verifying.ErrInvalidIndex
	if errors.As(err, &invalidIdx) {
		h.log.WithContext(ctx).With().Info("ATX with invalid post index", atx.ID(), log.Int("index", invalidIdx.Index))
		proof := &mwire.MalfeasanceProof{
			Layer: atx.PublishEpoch.FirstLayer(),
			Proof: mwire.Proof{
				Type: mwire.InvalidPostIndex,
				Data: &mwire.InvalidPostIndexProof{
					Atx:        *atx,
					InvalidIdx: uint32(invalidIdx.Index),
				},
			},
		}
		encodedProof := codec.MustEncode(proof)
		if err := identities.SetMalicious(h.cdb, atx.SmesherID, encodedProof, time.Now()); err != nil {
			return 0, 0, nil, fmt.Errorf("adding malfeasance proof: %w", err)
		}
		h.cdb.CacheMalfeasanceProof(atx.SmesherID, proof)
		h.tortoise.OnMalfeasance(atx.SmesherID)
		return 0, 0, proof, nil
	}
	if err != nil {
		return 0, 0, nil, fmt.Errorf("invalid nipost: %w", err)
	}

	return leaves, effectiveNumUnits, nil, err
}

func (h *Handler) validateNonInitialAtx(
	ctx context.Context,
	atx *wire.ActivationTxV1,
	commitmentATX types.ATXID,
) (uint32, error) {
	if err := h.nipostValidator.NIPostChallengeV1(&atx.NIPostChallengeV1, h.cdb, atx.SmesherID); err != nil {
		return 0, err
	}

	prevAtx, err := h.cdb.GetAtxHeader(atx.PrevATXID)
	if err != nil {
		return 0, err
	}

	nonce := atx.VRFNonce
	if atx.NumUnits > prevAtx.NumUnits && nonce == nil {
		h.log.WithContext(ctx).With().Info("post size increased without new vrf Nonce, re-validating current nonce",
			atx.ID(),
			log.Stringer("smesher", atx.SmesherID),
		)

		// This is not expected to happen very often, so we query the database
		// directly here without using the cache.
		current, err := atxs.NonceByID(h.cdb, prevAtx.ID)
		if err != nil {
			return 0, fmt.Errorf("failed to get current nonce: %w", err)
		}
		nonce = (*uint64)(&current)
	}

	if nonce != nil {
		err = h.nipostValidator.
			VRFNonce(atx.SmesherID, commitmentATX, *nonce, atx.NIPost.PostMetadata.LabelsPerUnit, atx.NumUnits)
		if err != nil {
			return 0, fmt.Errorf("invalid vrf nonce: %w", err)
		}
	}

	return min(prevAtx.NumUnits, atx.NumUnits), nil
}

// contextuallyValidateAtx ensures that the previous ATX referenced is the last known ATX for the referenced miner ID.
// If a previous ATX is not referenced, it validates that indeed there's no previous known ATX for that miner ID.
func (h *Handler) contextuallyValidateAtx(atx *wire.ActivationTxV1) error {
	lastAtx, err := atxs.GetLastIDByNodeID(h.cdb, atx.SmesherID)
	if err == nil && atx.PrevATXID == lastAtx {
		// last atx referenced equals last ATX seen from node
		return nil
	}

	if err == nil && atx.PrevATXID == types.EmptyATXID {
		// no previous atx declared, but already seen at least one atx from node
		return fmt.Errorf(
			"no prev atx reported, but other atx with same node id (%v) found: %v",
			atx.SmesherID,
			lastAtx.ShortString(),
		)
	}

	if err == nil && atx.PrevATXID != lastAtx {
		// last atx referenced does not equal last ATX seen from node
		return errors.New("last atx is not the one referenced")
	}

	if errors.Is(err, sql.ErrNotFound) && atx.PrevATXID == types.EmptyATXID {
		// no previous atx found and none referenced
		return nil
	}

	if err != nil && atx.PrevATXID != types.EmptyATXID {
		// no previous atx found but previous atx referenced
		h.log.With().Error("could not fetch node last atx",
			atx.ID(),
			log.Stringer("smesher", atx.SmesherID),
			log.Err(err),
		)
		return fmt.Errorf("could not fetch node last atx: %w", err)
	}

	return err
}

// cacheAtx caches the atx in the atxsdata cache.
// Returns true if the atx was cached, false otherwise.
func (h *Handler) cacheAtx(ctx context.Context, atx *types.ActivationTxHeader, nonce types.VRFPostIndex) *atxsdata.ATX {
	if !h.atxsdata.IsEvicted(atx.TargetEpoch()) {
		malicious, err := h.cdb.IsMalicious(atx.NodeID)
		if err != nil {
			h.log.With().Error("failed is malicious read", log.Err(err), log.Context(ctx))
			return nil
		}
		return h.atxsdata.AddFromHeader(atx, nonce, malicious)
	}
	return nil
}

// storeAtx stores an ATX and notifies subscribers of the ATXID.
func (h *Handler) storeAtx(ctx context.Context, atx *types.VerifiedActivationTx) (*mwire.MalfeasanceProof, error) {
	var nonce *types.VRFPostIndex
	malicious, err := h.cdb.IsMalicious(atx.SmesherID)
	if err != nil {
		return nil, fmt.Errorf("checking if node is malicious: %w", err)
	}
	var proof *mwire.MalfeasanceProof
	if err := h.cdb.WithTx(ctx, func(tx *sql.Tx) error {
		if malicious {
			if err := atxs.Add(tx, atx); err != nil && !errors.Is(err, sql.ErrObjectExists) {
				return fmt.Errorf("add atx to db: %w", err)
			}
			return nil
		}

		prev, err := atxs.GetByEpochAndNodeID(tx, atx.PublishEpoch, atx.SmesherID)
		if err != nil && !errors.Is(err, sql.ErrNotFound) {
			return err
		}

		// do ID check to be absolutely sure.
		if prev != nil && prev.ID() != atx.ID() {
			if _, ok := h.signers[atx.SmesherID]; ok {
				// if we land here we tried to publish 2 ATXs in the same epoch
				// don't punish ourselves but fail validation and thereby the handling of the incoming ATX
				return fmt.Errorf("%s already published an ATX in epoch %d", atx.SmesherID.ShortString(),
					atx.PublishEpoch,
				)
			}

			var atxProof mwire.AtxProof
			for i, a := range []*types.VerifiedActivationTx{prev, atx} {
				atxProof.Messages[i] = mwire.AtxProofMsg{
					InnerMsg: types.ATXMetadata{
						PublishEpoch: a.PublishEpoch,
						MsgHash:      a.ActivationTx.ID().Hash32(),
					},
					SmesherID: a.SmesherID,
					Signature: a.Signature,
				}
			}
			proof = &mwire.MalfeasanceProof{
				Layer: atx.PublishEpoch.FirstLayer(),
				Proof: mwire.Proof{
					Type: mwire.MultipleATXs,
					Data: &atxProof,
				},
			}
			encoded, err := codec.Encode(proof)
			if err != nil {
				h.log.With().Panic("failed to encode malfeasance proof", log.Err(err))
			}
			if err := identities.SetMalicious(tx, atx.SmesherID, encoded, time.Now()); err != nil {
				return fmt.Errorf("add malfeasance proof: %w", err)
			}

			h.log.WithContext(ctx).With().Warning("smesher produced more than one atx in the same epoch",
				log.Stringer("smesher", atx.SmesherID),
				log.Object("prev", prev),
				log.Object("curr", atx),
			)
		}

		nonce, err = atxs.AddGettingNonce(tx, atx)
		if err != nil && !errors.Is(err, sql.ErrObjectExists) {
			return fmt.Errorf("add atx to db: %w", err)
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("store atx: %w", err)
	}
	if nonce == nil {
		return nil, errors.New("no nonce")
	}
	atxs.AtxAdded(h.cdb, atx)
	if proof != nil {
		h.cdb.CacheMalfeasanceProof(atx.SmesherID, proof)
		h.tortoise.OnMalfeasance(atx.SmesherID)
	}
	header := atx.ToHeader()
	added := h.cacheAtx(ctx, header, *nonce)
	h.beacon.OnAtx(header)
	if added != nil {
		h.tortoise.OnAtx(atx.TargetEpoch(), atx.ID(), added)
	}

	h.log.WithContext(ctx).With().Debug("finished storing atx in epoch", atx.ID(), atx.PublishEpoch)

	return proof, nil
}

// HandleSyncedAtx handles atxs received by sync.
func (h *Handler) HandleSyncedAtx(ctx context.Context, expHash types.Hash32, peer p2p.Peer, data []byte) error {
	_, err := h.handleAtx(ctx, expHash, peer, data)
	if err != nil && !errors.Is(err, errMalformedData) && !errors.Is(err, errKnownAtx) {
		h.log.WithContext(ctx).With().Warning("failed to process synced atx",
			log.Stringer("sender", peer),
			log.Err(err),
		)
	}
	if errors.Is(err, errKnownAtx) {
		return nil
	}
	return err
}

// HandleGossipAtx handles the atx gossip data channel.
func (h *Handler) HandleGossipAtx(ctx context.Context, peer p2p.Peer, msg []byte) error {
	proof, err := h.handleAtx(ctx, types.Hash32{}, peer, msg)
	if err != nil && !errors.Is(err, errMalformedData) && !errors.Is(err, errKnownAtx) {
		h.log.WithContext(ctx).With().Warning("failed to process atx gossip",
			log.Stringer("sender", peer),
			log.Err(err),
		)
	}
	if errors.Is(err, errKnownAtx) && peer == h.local {
		return nil
	}

	// broadcast malfeasance proof last as the verification of the proof will take place
	// in the same goroutine
	if proof != nil {
		gossip := mwire.MalfeasanceGossip{
			MalfeasanceProof: *proof,
		}
		encodedProof := codec.MustEncode(&gossip)
		if err = h.publisher.Publish(ctx, pubsub.MalfeasanceProof, encodedProof); err != nil {
			h.log.With().Error("failed to broadcast malfeasance proof", log.Err(err))
			return fmt.Errorf("broadcast atx malfeasance proof: %w", err)
		}
		return errMaliciousATX
	}
	return err
}

func (h *Handler) handleAtx(
	ctx context.Context,
	expHash types.Hash32,
	peer p2p.Peer,
	msg []byte,
) (*mwire.MalfeasanceProof, error) {
	receivedTime := time.Now()

	var atx wire.ActivationTxV1
	if err := codec.Decode(msg, &atx); err != nil {
		return nil, fmt.Errorf("%w: %w", errMalformedData, err)
	}
	id := atx.ID()
	if (expHash != types.Hash32{}) && id.Hash32() != expHash {
		return nil, fmt.Errorf("%w: atx want %s, got %s", errWrongHash, expHash.ShortString(), id.ShortString())
	}

	// Check if processing is already in progress
	h.inProgressMu.Lock()
	if sub, ok := h.inProgress[id]; ok {
		ch := make(chan error, 1)
		h.inProgress[id] = append(sub, ch)
		h.inProgressMu.Unlock()
		h.log.WithContext(ctx).With().Debug("atx is already being processed. waiting for result", id)
		select {
		case err := <-ch:
			h.log.WithContext(ctx).With().Debug("atx processed in other task", id, log.Err(err))
			return nil, err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	h.inProgress[id] = []chan error{}
	h.inProgressMu.Unlock()
	h.log.WithContext(ctx).With().Info("handling incoming atx", id, log.Int("size", len(msg)))

	proof, err := h.processATX(ctx, peer, atx, msg, receivedTime)
	h.inProgressMu.Lock()
	defer h.inProgressMu.Unlock()
	for _, ch := range h.inProgress[id] {
		ch <- err
		close(ch)
	}
	delete(h.inProgress, id)
	return proof, err
}

func (h *Handler) processATX(
	ctx context.Context,
	peer p2p.Peer,
	watx wire.ActivationTxV1,
	blob []byte,
	received time.Time,
) (*mwire.MalfeasanceProof, error) {
	if !h.edVerifier.Verify(signing.ATX, watx.SmesherID, watx.SignedBytes(), watx.Signature) {
		return nil, fmt.Errorf("invalid atx signature: %w", errMalformedData)
	}

	existing, _ := h.cdb.GetAtxHeader(watx.ID())
	if existing != nil {
		return nil, fmt.Errorf("%w atx %s", errKnownAtx, watx.ID())
	}

	h.log.WithContext(ctx).With().
		Debug("processing atx", watx.ID(), watx.PublishEpoch, log.Stringer("smesherID", watx.SmesherID))

	if err := h.syntacticallyValidate(ctx, &watx); err != nil {
		return nil, fmt.Errorf("atx %s syntactically invalid: %w", watx.ID(), err)
	}

	poetRef, atxIDs := collectAtxDeps(h.goldenATXID, &watx)
	h.registerHashes(peer, poetRef, atxIDs)
	if err := h.fetchReferences(ctx, poetRef, atxIDs); err != nil {
		return nil, fmt.Errorf("fetching references for atx %s: %w", watx.ID(), err)
	}

	leaves, effectiveNumUnits, proof, err := h.syntacticallyValidateDeps(ctx, &watx)
	if err != nil {
		return nil, fmt.Errorf("atx %s syntactically invalid based on deps: %w", watx.ID(), err)
	}

	if proof != nil {
		return proof, err
	}

	if err := h.contextuallyValidateAtx(&watx); err != nil {
		h.log.WithContext(ctx).With().
			Warning("atx is contextually invalid ", watx.ID(), log.Stringer("smesherID", watx.SmesherID), log.Err(err))
	} else {
		h.log.WithContext(ctx).With().Debug("atx is valid", watx.ID())
	}

	var baseTickHeight uint64
	if watx.PositioningATXID != h.goldenATXID {
		posAtx, err := h.cdb.GetAtxHeader(watx.PositioningATXID)
		if err != nil {
			return nil, fmt.Errorf("failed to get positioning atx %s: %w", watx.PositioningATXID, err)
		}
		baseTickHeight = posAtx.TickHeight()
	}

	atx := wire.ActivationTxFromWireV1(&watx, blob...)
	if h.nipostValidator.IsVerifyingFullPost() {
		atx.SetValidity(types.Valid)
	}
	atx.SetReceived(received)
	atx.SetEffectiveNumUnits(effectiveNumUnits)
	vAtx, err := atx.Verify(baseTickHeight, leaves/h.tickSize)
	if err != nil {
		return nil, fmt.Errorf("failed to verify atx %x: %w", watx.ID(), err)
	}

	proof, err = h.storeAtx(ctx, vAtx)
	if err != nil {
		return nil, fmt.Errorf("cannot store atx %s: %w", atx.ShortString(), err)
	}

	events.ReportNewActivation(vAtx)
	h.log.WithContext(ctx).With().Info("new atx", log.Inline(vAtx), log.Bool("malicious", proof != nil))
	return proof, err
}

// registerHashes registers that the given peer should be asked for
// the hashes of the poet proof and ATXs.
func (h *Handler) registerHashes(peer p2p.Peer, poetRef types.Hash32, atxIDs []types.ATXID) {
	hashes := make([]types.Hash32, 0, len(atxIDs)+1)
	for _, id := range atxIDs {
		hashes = append(hashes, id.Hash32())
	}
	hashes = append(hashes, types.Hash32(poetRef))
	h.fetcher.RegisterPeerHashes(peer, hashes)
}

// fetchReferences makes sure that the referenced poet proof and ATXs are available.
func (h *Handler) fetchReferences(ctx context.Context, poetRef types.Hash32, atxIDs []types.ATXID) error {
	if err := h.fetcher.GetPoetProof(ctx, poetRef); err != nil {
		return fmt.Errorf("missing poet proof (%s): %w", poetRef.ShortString(), err)
	}

	if len(atxIDs) == 0 {
		return nil
	}

	if err := h.fetcher.GetAtxs(ctx, atxIDs, system.WithoutLimiting()); err != nil {
		return fmt.Errorf("missing atxs %x: %w", atxIDs, err)
	}

	h.log.WithContext(ctx).With().Debug("done fetching references", log.Int("fetched", len(atxIDs)))
	return nil
}

// Collect unique dependencies of an ATX.
// Filters out EmptyATXID and the golden ATX.
func collectAtxDeps(goldenAtxId types.ATXID, atx *wire.ActivationTxV1) (types.Hash32, []types.ATXID) {
	ids := []types.ATXID{atx.PrevATXID, atx.PositioningATXID}
	if atx.CommitmentATXID != nil {
		ids = append(ids, *atx.CommitmentATXID)
	}

	filtered := make(map[types.ATXID]struct{})
	for _, id := range ids {
		if id != types.EmptyATXID && id != goldenAtxId {
			filtered[id] = struct{}{}
		}
	}

	return types.BytesToHash(atx.NIPost.PostMetadata.Challenge), maps.Keys(filtered)
}

// Code generated by github.com/spacemeshos/go-scale/scalegen. DO NOT EDIT.

// nolint
package wire

import (
	"github.com/spacemeshos/go-scale"
	"github.com/spacemeshos/go-spacemesh/common/types"
)

func (t *MalfeasanceProof) EncodeScale(enc *scale.Encoder) (total int, err error) {
	{
		n, err := scale.EncodeCompact32(enc, uint32(t.Layer))
		if err != nil {
			return total, err
		}
		total += n
	}
	{
		n, err := t.Proof.EncodeScale(enc)
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

func (t *MalfeasanceProof) DecodeScale(dec *scale.Decoder) (total int, err error) {
	{
		field, n, err := scale.DecodeCompact32(dec)
		if err != nil {
			return total, err
		}
		total += n
		t.Layer = types.LayerID(field)
	}
	{
		n, err := t.Proof.DecodeScale(dec)
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

func (t *MalfeasanceGossip) EncodeScale(enc *scale.Encoder) (total int, err error) {
	{
		n, err := t.MalfeasanceProof.EncodeScale(enc)
		if err != nil {
			return total, err
		}
		total += n
	}
	{
		n, err := scale.EncodeOption(enc, t.Eligibility)
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

func (t *MalfeasanceGossip) DecodeScale(dec *scale.Decoder) (total int, err error) {
	{
		n, err := t.MalfeasanceProof.DecodeScale(dec)
		if err != nil {
			return total, err
		}
		total += n
	}
	{
		field, n, err := scale.DecodeOption[types.HareEligibilityGossip](dec)
		if err != nil {
			return total, err
		}
		total += n
		t.Eligibility = field
	}
	return total, nil
}

func (t *AtxProof) EncodeScale(enc *scale.Encoder) (total int, err error) {
	{
		n, err := scale.EncodeStructArray(enc, t.Messages[:])
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

func (t *AtxProof) DecodeScale(dec *scale.Decoder) (total int, err error) {
	{
		n, err := scale.DecodeStructArray(dec, t.Messages[:])
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

func (t *BallotProof) EncodeScale(enc *scale.Encoder) (total int, err error) {
	{
		n, err := scale.EncodeStructArray(enc, t.Messages[:])
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

func (t *BallotProof) DecodeScale(dec *scale.Decoder) (total int, err error) {
	{
		n, err := scale.DecodeStructArray(dec, t.Messages[:])
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

func (t *HareProof) EncodeScale(enc *scale.Encoder) (total int, err error) {
	{
		n, err := scale.EncodeStructArray(enc, t.Messages[:])
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

func (t *HareProof) DecodeScale(dec *scale.Decoder) (total int, err error) {
	{
		n, err := scale.DecodeStructArray(dec, t.Messages[:])
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

func (t *AtxProofMsg) EncodeScale(enc *scale.Encoder) (total int, err error) {
	{
		n, err := t.InnerMsg.EncodeScale(enc)
		if err != nil {
			return total, err
		}
		total += n
	}
	{
		n, err := scale.EncodeByteArray(enc, t.SmesherID[:])
		if err != nil {
			return total, err
		}
		total += n
	}
	{
		n, err := scale.EncodeByteArray(enc, t.Signature[:])
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

func (t *AtxProofMsg) DecodeScale(dec *scale.Decoder) (total int, err error) {
	{
		n, err := t.InnerMsg.DecodeScale(dec)
		if err != nil {
			return total, err
		}
		total += n
	}
	{
		n, err := scale.DecodeByteArray(dec, t.SmesherID[:])
		if err != nil {
			return total, err
		}
		total += n
	}
	{
		n, err := scale.DecodeByteArray(dec, t.Signature[:])
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

func (t *BallotProofMsg) EncodeScale(enc *scale.Encoder) (total int, err error) {
	{
		n, err := t.InnerMsg.EncodeScale(enc)
		if err != nil {
			return total, err
		}
		total += n
	}
	{
		n, err := scale.EncodeByteArray(enc, t.SmesherID[:])
		if err != nil {
			return total, err
		}
		total += n
	}
	{
		n, err := scale.EncodeByteArray(enc, t.Signature[:])
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

func (t *BallotProofMsg) DecodeScale(dec *scale.Decoder) (total int, err error) {
	{
		n, err := t.InnerMsg.DecodeScale(dec)
		if err != nil {
			return total, err
		}
		total += n
	}
	{
		n, err := scale.DecodeByteArray(dec, t.SmesherID[:])
		if err != nil {
			return total, err
		}
		total += n
	}
	{
		n, err := scale.DecodeByteArray(dec, t.Signature[:])
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

func (t *HareProofMsg) EncodeScale(enc *scale.Encoder) (total int, err error) {
	{
		n, err := t.InnerMsg.EncodeScale(enc)
		if err != nil {
			return total, err
		}
		total += n
	}
	{
		n, err := scale.EncodeByteArray(enc, t.SmesherID[:])
		if err != nil {
			return total, err
		}
		total += n
	}
	{
		n, err := scale.EncodeByteArray(enc, t.Signature[:])
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

func (t *HareProofMsg) DecodeScale(dec *scale.Decoder) (total int, err error) {
	{
		n, err := t.InnerMsg.DecodeScale(dec)
		if err != nil {
			return total, err
		}
		total += n
	}
	{
		n, err := scale.DecodeByteArray(dec, t.SmesherID[:])
		if err != nil {
			return total, err
		}
		total += n
	}
	{
		n, err := scale.DecodeByteArray(dec, t.Signature[:])
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

func (t *HareMetadata) EncodeScale(enc *scale.Encoder) (total int, err error) {
	{
		n, err := scale.EncodeCompact32(enc, uint32(t.Layer))
		if err != nil {
			return total, err
		}
		total += n
	}
	{
		n, err := scale.EncodeCompact32(enc, uint32(t.Round))
		if err != nil {
			return total, err
		}
		total += n
	}
	{
		n, err := scale.EncodeByteArray(enc, t.MsgHash[:])
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

func (t *HareMetadata) DecodeScale(dec *scale.Decoder) (total int, err error) {
	{
		field, n, err := scale.DecodeCompact32(dec)
		if err != nil {
			return total, err
		}
		total += n
		t.Layer = types.LayerID(field)
	}
	{
		field, n, err := scale.DecodeCompact32(dec)
		if err != nil {
			return total, err
		}
		total += n
		t.Round = uint32(field)
	}
	{
		n, err := scale.DecodeByteArray(dec, t.MsgHash[:])
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

func (t *InvalidPostIndexProof) EncodeScale(enc *scale.Encoder) (total int, err error) {
	{
		n, err := t.Atx.EncodeScale(enc)
		if err != nil {
			return total, err
		}
		total += n
	}
	{
		n, err := scale.EncodeCompact32(enc, uint32(t.InvalidIdx))
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

func (t *InvalidPostIndexProof) DecodeScale(dec *scale.Decoder) (total int, err error) {
	{
		n, err := t.Atx.DecodeScale(dec)
		if err != nil {
			return total, err
		}
		total += n
	}
	{
		field, n, err := scale.DecodeCompact32(dec)
		if err != nil {
			return total, err
		}
		total += n
		t.InvalidIdx = uint32(field)
	}
	return total, nil
}

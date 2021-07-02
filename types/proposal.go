package types

import (
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/dashevo/dashd-go/btcjson"
	"github.com/tendermint/tendermint/crypto"
	"github.com/tendermint/tendermint/crypto/bls12381"
	"math"
	"time"

	tmbytes "github.com/tendermint/tendermint/libs/bytes"
	"github.com/tendermint/tendermint/libs/protoio"
	tmproto "github.com/tendermint/tendermint/proto/tendermint/types"
	tmtime "github.com/tendermint/tendermint/types/time"
)

var (
	ErrInvalidBlockPartSignature = errors.New("error invalid block part signature")
	ErrInvalidBlockPartHash      = errors.New("error invalid block part hash")
)

// Proposal defines a block proposal for the consensus.
// It refers to the block by BlockID field.
// It must be signed by the correct proposer for the given Height/Round
// to be considered valid. It may depend on votes from a previous round,
// a so-called Proof-of-Lock (POL) round, as noted in the POLRound.
// If POLRound >= 0, then BlockID corresponds to the block that is locked in POLRound.
type Proposal struct {
	Type                  tmproto.SignedMsgType
	Height                int64     `json:"height"`
	CoreChainLockedHeight uint32    `json:"core_height"`
	Round                 int32     `json:"round"`     // there can not be greater than 2_147_483_647 rounds
	POLRound              int32     `json:"pol_round"` // -1 if null.
	BlockID               BlockID   `json:"block_id"`
	Timestamp             time.Time `json:"timestamp"`
	Signature             []byte    `json:"signature"`
}

// NewProposal returns a new Proposal.
// If there is no POLRound, polRound should be -1.
func NewProposal(height int64, coreChainLockedHeight uint32, round int32, polRound int32, blockID BlockID) *Proposal {
	return &Proposal{
		Type:                  tmproto.ProposalType,
		Height:                height,
		CoreChainLockedHeight: coreChainLockedHeight,
		Round:                 round,
		BlockID:               blockID,
		POLRound:              polRound,
		Timestamp:             tmtime.Now(),
	}
}

// ValidateBasic performs basic validation.
func (p *Proposal) ValidateBasic() error {
	if p.Type != tmproto.ProposalType {
		return errors.New("invalid Type")
	}
	if p.Height < 0 {
		return errors.New("negative Height")
	}
	if p.CoreChainLockedHeight == math.MaxUint32 {
		return errors.New("core height not set")
	}
	if p.Round < 0 {
		return errors.New("negative Round")
	}
	if p.POLRound < -1 {
		return errors.New("negative POLRound (exception: -1)")
	}
	if err := p.BlockID.ValidateBasic(); err != nil {
		return fmt.Errorf("wrong BlockID: %v", err)
	}
	// ValidateBasic above would pass even if the BlockID was empty:
	if !p.BlockID.IsComplete() {
		return fmt.Errorf("expected a complete, non-empty BlockID, got: %v", p.BlockID)
	}

	// NOTE: Timestamp validation is subtle and handled elsewhere.

	if len(p.Signature) == 0 {
		return errors.New("signature is missing")
	}

	if len(p.Signature) > SignatureSize {
		return fmt.Errorf("signature is too big (max: %d)", SignatureSize)
	}
	return nil
}

// String returns a string representation of the Proposal.
//
// 1. height
// 2. round
// 3. block ID
// 4. POL round
// 5. first 6 bytes of signature
// 6. timestamp
//
// See BlockID#String.
func (p *Proposal) String() string {
	if p == nil {
		return fmt.Sprintf("Proposal{nil}")
	}
	return fmt.Sprintf("Proposal{%v/%v (%v, %v) %X @ %s}",
		p.Height,
		p.Round,
		p.BlockID,
		p.POLRound,
		tmbytes.Fingerprint(p.Signature),
		CanonicalTime(p.Timestamp))
}

// ProposalBlockSignBytes returns the proto-encoding of the canonicalized Proposal,
// for signing. Panics if the marshaling fails.
//
// The encoded Protobuf message is varint length-prefixed (using MarshalDelimited)
// for backwards-compatibility with the Amino encoding, due to e.g. hardware
// devices that rely on this encoding.
//
// See CanonicalizeProposal
func ProposalBlockSignBytes(chainID string, p *tmproto.Proposal) []byte {
	pb := CanonicalizeProposal(chainID, p)
	bz, err := protoio.MarshalDelimited(&pb)
	if err != nil {
		panic(err)
	}

	return bz
}

func ProposalBlockSignId(chainID string, p *tmproto.Proposal, quorumType btcjson.LLMQType, quorumHash crypto.QuorumHash) []byte {
	signBytes := ProposalBlockSignBytes(chainID, p)
	proposalMessageHash := crypto.Sha256(signBytes)

	proposalRequestId := ProposalRequestIdProto(p)

	signId := crypto.SignId(quorumType, bls12381.ReverseBytes(quorumHash), bls12381.ReverseBytes(proposalRequestId), bls12381.ReverseBytes(proposalMessageHash))

	return signId
}

func ProposalRequestId(p *Proposal) []byte {
	requestIdMessage := []byte("dpproposal")
	heightByteArray := make([]byte, 8)
	binary.LittleEndian.PutUint64(heightByteArray, uint64(p.Height))
	roundByteArray := make([]byte, 4)
	binary.LittleEndian.PutUint32(roundByteArray, uint32(p.Round))

	requestIdMessage = append(requestIdMessage, heightByteArray...)
	requestIdMessage = append(requestIdMessage, roundByteArray...)

	return crypto.Sha256(requestIdMessage)
}

func ProposalRequestIdProto(p *tmproto.Proposal) []byte {
	requestIdMessage := []byte("dpproposal")
	heightByteArray := make([]byte, 8)
	binary.LittleEndian.PutUint64(heightByteArray, uint64(p.Height))
	roundByteArray := make([]byte, 4)
	binary.LittleEndian.PutUint32(roundByteArray, uint32(p.Round))

	requestIdMessage = append(requestIdMessage, heightByteArray...)
	requestIdMessage = append(requestIdMessage, roundByteArray...)

	return crypto.Sha256(requestIdMessage)
}

// ToProto converts Proposal to protobuf
func (p *Proposal) ToProto() *tmproto.Proposal {
	if p == nil {
		return &tmproto.Proposal{}
	}
	pb := new(tmproto.Proposal)

	pb.BlockID = p.BlockID.ToProto()
	pb.Type = p.Type
	pb.Height = p.Height
	pb.CoreChainLockedHeight = p.CoreChainLockedHeight
	pb.Round = p.Round
	pb.PolRound = p.POLRound
	pb.Timestamp = p.Timestamp
	pb.Signature = p.Signature

	return pb
}

// FromProto sets a protobuf Proposal to the given pointer.
// It returns an error if the proposal is invalid.
func ProposalFromProto(pp *tmproto.Proposal) (*Proposal, error) {
	if pp == nil {
		return nil, errors.New("nil proposal")
	}

	p := new(Proposal)

	blockID, err := BlockIDFromProto(&pp.BlockID)
	if err != nil {
		return nil, err
	}

	p.BlockID = *blockID
	p.Type = pp.Type
	p.Height = pp.Height
	p.CoreChainLockedHeight = pp.CoreChainLockedHeight
	p.Round = pp.Round
	p.POLRound = pp.PolRound
	p.Timestamp = pp.Timestamp
	p.Signature = pp.Signature

	return p, p.ValidateBasic()
}

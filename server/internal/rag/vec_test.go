package rag

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVecCodecRoundtrip(t *testing.T) {
	v := []float32{0.5, -1.25, 3.75, 0, 1e-7}
	got := DecodeVec(EncodeVec(v))
	assert.InDeltaSlice(t, v, got, 1e-9, "往返无损")
	assert.Equal(t, len(v)*4, len(EncodeVec(v)), "每维 4 字节定长")
}

func TestDecodeVecRejectsMalformed(t *testing.T) {
	assert.Nil(t, DecodeVec(nil))
	assert.Nil(t, DecodeVec([]byte{1, 2, 3}), "非 4 倍数")
	assert.NotNil(t, DecodeVec(make([]byte, 8)))
}

func TestVecCodecOrder(t *testing.T) {
	v := []float32{math.MaxFloat32, -2}
	got := DecodeVec(EncodeVec(v))
	assert.Equal(t, v[0], got[0])
	assert.Equal(t, v[1], got[1])
}

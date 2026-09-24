package service

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestRealtimeCycleAdmissionExcludesControlEvents(t *testing.T) {
	for _, tc := range []struct {
		payload string
		want    bool
	}{
		{`{"type":"session.update","audio":"aGVsbG8="}`, false},
		{`{"type":"input_audio_buffer.clear","audio":"aGVsbG8="}`, false},
		{`{"type":"response.cancel","audio":"aGVsbG8="}`, false},
		{`{"type":"unknown.audio","audio":"aGVsbG8="}`, false},
		{`{"type":"input_audio_buffer.append","audio":""}`, false},
		{`{"type":"input_audio_buffer.append","audio":"aGVsbG8="}`, true},
		{`{"type":"response.create"}`, true},
		{"{", false},
	} {
		require.Equal(t, tc.want, grokRealtimeEventStartsGeneration([]byte(tc.payload)), tc.payload)
	}
}

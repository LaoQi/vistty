package replay

import (
	_ "embed"
)

//go:embed recording_nvim_full.bin
var nvimRecording []byte

func loadEmbeddedRecording() []byte {
	if len(nvimRecording) > 0 {
		return nvimRecording
	}
	return nil
}

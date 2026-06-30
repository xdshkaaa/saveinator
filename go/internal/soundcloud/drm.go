package soundcloud

import (
	"strings"

	"saveinator/internal/audio"
)

func isDRMError(err error, output []byte) bool {
	return audio.IsDRMProtectedError(err, string(output))
}

func execOutputMessage(err error, output []byte) string {
	var b strings.Builder
	if len(output) > 0 {
		b.Write(output)
	}
	if err != nil {
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(err.Error())
	}
	return b.String()
}

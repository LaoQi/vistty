package session

import "github.com/LaoQi/vistty/internal/platform"

type InputTarget interface {
	HandleKey(ev platform.KeyEvent)
	CommitText(text string)
}

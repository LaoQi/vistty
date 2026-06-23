package drm

import (
	"fmt"

	drminternal "github.com/LaoQi/vistty/internal/platform/drm/internal"
)

type DisplayInfo struct {
	ConnectorID uint32
	CrtcID      uint32
	Mode        drminternal.ModeInfoPublic
	SavedCrtc   *drminternal.CrtcResult
}

func findDisplay(fd int) (*DisplayInfo, error) {
	res, err := drminternal.GetResources(fd)
	if err != nil {
		return nil, fmt.Errorf("get resources: %w", err)
	}

	for _, connID := range res.ConnectorIDs {
		conn, err := drminternal.GetConnector(fd, connID)
		if err != nil {
			continue
		}
		if conn.Connection != uint32(drminternal.Connected) {
			continue
		}
		if len(conn.Modes) == 0 {
			continue
		}

		enc, err := drminternal.GetEncoder(fd, conn.EncoderID)
		if err != nil {
			continue
		}

		crtcID := enc.CrtcID
		if crtcID == 0 {
			for _, cID := range res.CrtcIDs {
				if enc.PossibleCrtcs&(1<<crtcIndex(res.CrtcIDs, cID)) != 0 {
					crtcID = cID
					break
				}
			}
		}
		if crtcID == 0 {
			continue
		}

		savedCrtc, err := drminternal.GetCrtc(fd, crtcID)
		if err != nil {
			continue
		}

		return &DisplayInfo{
			ConnectorID: connID,
			CrtcID:      crtcID,
			Mode:        conn.Modes[0],
			SavedCrtc:   savedCrtc,
		}, nil
	}

	return nil, fmt.Errorf("no connected display found")
}

func crtcIndex(crtcIDs []uint32, id uint32) uint32 {
	for i, cID := range crtcIDs {
		if cID == id {
			return uint32(i)
		}
	}
	return 0
}

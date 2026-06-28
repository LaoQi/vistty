package drm

import (
	"fmt"

	"github.com/LaoQi/vistty/internal/platform"
)

const modeTypePreferred = 1 << 3

var connectorTypeNames = map[uint32]string{
	0:  "Unknown",
	1:  "VGA",
	2:  "DVI-I",
	3:  "DVI-D",
	4:  "DVI-A",
	5:  "Composite",
	6:  "SVIDEO",
	7:  "LVDS",
	8:  "Component",
	9:  "DIN",
	10: "DP",
	11: "HDMI-A",
	12: "HDMI-B",
	13: "TV",
	14: "eDP",
	15: "Virtual",
	16: "DSI",
	17: "DPI",
	18: "Writeback",
	19: "SPI",
	20: "USB",
}

type DisplayInfo struct {
	connID    uint32
	crtcID    uint32
	mode      ModeInfoPublic
	savedCrtc *CrtcResult
	name      string
}

func (d *DisplayInfo) ID() uint32          { return d.connID }
func (d *DisplayInfo) ConnectorID() uint32 { return d.connID }
func (d *DisplayInfo) CrtcID() uint32       { return d.crtcID }
func (d *DisplayInfo) Name() string        { return d.name }
func (d *DisplayInfo) Size() (int, int)        { return int(d.mode.HDisplay), int(d.mode.VDisplay) }
func (d *DisplayInfo) ModeInfo() ModeInfoPublic { return d.mode }

var _ platform.Output = (*DisplayInfo)(nil)

func connectorName(connType, typeID uint32) string {
	prefix, ok := connectorTypeNames[connType]
	if !ok {
		prefix = "Unknown"
	}
	return fmt.Sprintf("%s-%d", prefix, typeID)
}

func findOutputs(fd int) ([]*DisplayInfo, error) {
	res, err := GetResources(fd)
	if err != nil {
		return nil, fmt.Errorf("get resources: %w", err)
	}

	var outputs []*DisplayInfo

	for _, connID := range res.ConnectorIDs {
		conn, err := GetConnector(fd, connID)
		if err != nil {
			continue
		}
		if conn.Connection != uint32(Connected) {
			continue
		}
		if len(conn.Modes) == 0 {
			continue
		}

		enc, err := GetEncoder(fd, conn.EncoderID)
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

		savedCrtc, err := GetCrtc(fd, crtcID)
		if err != nil {
			continue
		}

		mode := conn.Modes[0]
		for _, m := range conn.Modes {
			if m.Type&modeTypePreferred != 0 {
				mode = m
				break
			}
		}

		outputs = append(outputs, &DisplayInfo{
			connID:    connID,
			crtcID:    crtcID,
			mode:      mode,
			savedCrtc: savedCrtc,
			name:      connectorName(conn.ConnectorType, conn.TypeID),
		})
	}

	if len(outputs) == 0 {
		return nil, fmt.Errorf("no connected display found")
	}

	return outputs, nil
}

func findPrimaryDisplay(fd int, primaryName string) (*DisplayInfo, error) {
	outputs, err := findOutputs(fd)
	if err != nil {
		return nil, err
	}

	if primaryName != "" {
		for _, o := range outputs {
			if o.Name() == primaryName {
				return o, nil
			}
		}
	}

	return outputs[0], nil
}

func crtcIndex(crtcIDs []uint32, id uint32) uint32 {
	for i, cID := range crtcIDs {
		if cID == id {
			return uint32(i)
		}
	}
	return 0
}

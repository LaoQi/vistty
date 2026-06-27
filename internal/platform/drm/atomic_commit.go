package drm

import (
	"fmt"
	"sync"

	"github.com/LaoQi/vistty/internal/debug"
	drminternal "github.com/LaoQi/vistty/internal/platform/drm/internal"
)

type planeProps struct {
	fbID   uint32
	crtcID uint32
	srcX   uint32
	srcY   uint32
	srcW   uint32
	srcH   uint32
	crtcX  uint32
	crtcY  uint32
	crtcW  uint32
	crtcH  uint32
}

type crtcProps struct {
	active uint32
	modeID uint32
}

type connProps struct {
	crtcID uint32
}

type surfaceAtomicInfo struct {
	crtcID     uint32
	connectorID uint32
	planeID    uint32
	width      uint32
	height     uint32

	cProps crtcProps
	pProps planeProps
	aConn  connProps

	modeBlobID uint32
	modesetDone bool
}

type pendingFlip struct {
	info  *surfaceAtomicInfo
	fbID  uint32
	first bool
}

type AtomicCommitor struct {
	fd     int
	mu     sync.Mutex
	infos  map[uint32]*surfaceAtomicInfo
	pending map[uint32]*pendingFlip
}

func NewAtomicCommitor(fd int) *AtomicCommitor {
	return &AtomicCommitor{
		fd:      fd,
		infos:   make(map[uint32]*surfaceAtomicInfo),
		pending: make(map[uint32]*pendingFlip),
	}
}

func findPropID(fd int, objID, objType uint32, name string) (uint32, error) {
	propIDs, _, err := drminternal.GetObjectProperties(fd, objID, objType)
	if err != nil {
		return 0, fmt.Errorf("get properties for obj %d: %w", objID, err)
	}

	for _, pid := range propIDs {
		prop, err := drminternal.GetProperty(fd, pid)
		if err != nil {
			continue
		}
		if prop.Name == name {
			return pid, nil
		}
	}
	return 0, fmt.Errorf("property %q not found on object %d", name, objID)
}

func (c *AtomicCommitor) findPrimaryPlane(crtcID uint32) (uint32, error) {
	res, err := drminternal.GetResources(c.fd)
	if err != nil {
		return 0, fmt.Errorf("get resources: %w", err)
	}

	crtcIndex := uint32(0)
	found := false
	for i, id := range res.CrtcIDs {
		if id == crtcID {
			crtcIndex = uint32(i)
			found = true
			break
		}
	}
	if !found {
		return 0, fmt.Errorf("CRTC %d not found in resources", crtcID)
	}

	planeIDs, err := drminternal.GetPlaneResources(c.fd)
	if err != nil {
		return 0, fmt.Errorf("get plane resources: %w", err)
	}

	for _, pid := range planeIDs {
		plane, err := drminternal.GetPlane(c.fd, pid)
		if err != nil {
			continue
		}
		if plane.PossibleCrtcs&(1<<crtcIndex) == 0 {
			continue
		}

		propIDs, propValues, err := drminternal.GetObjectProperties(c.fd, pid, drminternal.ModeObjectPlane)
		if err != nil {
			continue
		}

		typePropID, err := findPropID(c.fd, pid, drminternal.ModeObjectPlane, "type")
		if err != nil {
			continue
		}

		for i, p := range propIDs {
			if p == typePropID && i < len(propValues) {
				debug.Debugf("findPrimaryPlane: plane %d type=%d for CRTC %d\n", pid, propValues[i], crtcID)
				if propValues[i] == drminternal.PlaneTypePrimary {
					return pid, nil
				}
			}
		}
	}

	return 0, fmt.Errorf("no primary plane found for CRTC %d", crtcID)
}

func (c *AtomicCommitor) Register(crtcID, connectorID uint32, width, height int, mode *drminternal.ModeInfoPublic) (*surfaceAtomicInfo, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if info, ok := c.infos[crtcID]; ok {
		return info, nil
	}

	planeID, err := c.findPrimaryPlane(crtcID)
	if err != nil {
		return nil, fmt.Errorf("find primary plane: %w", err)
	}

	info := &surfaceAtomicInfo{
		crtcID:     crtcID,
		connectorID: connectorID,
		planeID:    planeID,
		width:      uint32(width),
		height:     uint32(height),
	}

	info.cProps.active, err = findPropID(c.fd, crtcID, drminternal.ModeObjectCRTC, "ACTIVE")
	if err != nil {
		return nil, err
	}
	info.cProps.modeID, err = findPropID(c.fd, crtcID, drminternal.ModeObjectCRTC, "MODE_ID")
	if err != nil {
		return nil, err
	}

	info.pProps.fbID, err = findPropID(c.fd, planeID, drminternal.ModeObjectPlane, "FB_ID")
	if err != nil {
		return nil, err
	}
	info.pProps.crtcID, err = findPropID(c.fd, planeID, drminternal.ModeObjectPlane, "CRTC_ID")
	if err != nil {
		return nil, err
	}
	info.pProps.srcX, err = findPropID(c.fd, planeID, drminternal.ModeObjectPlane, "SRC_X")
	if err != nil {
		return nil, err
	}
	info.pProps.srcY, err = findPropID(c.fd, planeID, drminternal.ModeObjectPlane, "SRC_Y")
	if err != nil {
		return nil, err
	}
	info.pProps.srcW, err = findPropID(c.fd, planeID, drminternal.ModeObjectPlane, "SRC_W")
	if err != nil {
		return nil, err
	}
	info.pProps.srcH, err = findPropID(c.fd, planeID, drminternal.ModeObjectPlane, "SRC_H")
	if err != nil {
		return nil, err
	}
	info.pProps.crtcX, err = findPropID(c.fd, planeID, drminternal.ModeObjectPlane, "CRTC_X")
	if err != nil {
		return nil, err
	}
	info.pProps.crtcY, err = findPropID(c.fd, planeID, drminternal.ModeObjectPlane, "CRTC_Y")
	if err != nil {
		return nil, err
	}
	info.pProps.crtcW, err = findPropID(c.fd, planeID, drminternal.ModeObjectPlane, "CRTC_W")
	if err != nil {
		return nil, err
	}
	info.pProps.crtcH, err = findPropID(c.fd, planeID, drminternal.ModeObjectPlane, "CRTC_H")
	if err != nil {
		return nil, err
	}

	info.aConn.crtcID, err = findPropID(c.fd, connectorID, drminternal.ModeObjectConnector, "CRTC_ID")
	if err != nil {
		return nil, err
	}

	if mode != nil {
		blobID, err := drminternal.CreateModeBlob(c.fd, mode)
		if err != nil {
			return nil, fmt.Errorf("create mode blob: %w", err)
		}
		info.modeBlobID = blobID
	}

	c.infos[crtcID] = info
	return info, nil
}

func (c *AtomicCommitor) buildPlaneObject(info *surfaceAtomicInfo, fbID uint32) drminternal.AtomicObject {
	return drminternal.AtomicObject{
		ID: info.planeID,
		Props: []drminternal.AtomicProp{
			{ID: info.pProps.fbID, Value: uint64(fbID)},
			{ID: info.pProps.crtcID, Value: uint64(info.crtcID)},
			{ID: info.pProps.srcX, Value: 0},
			{ID: info.pProps.srcY, Value: 0},
			{ID: info.pProps.srcW, Value: uint64(info.width) << 16},
			{ID: info.pProps.srcH, Value: uint64(info.height) << 16},
			{ID: info.pProps.crtcX, Value: 0},
			{ID: info.pProps.crtcY, Value: 0},
			{ID: info.pProps.crtcW, Value: uint64(info.width)},
			{ID: info.pProps.crtcH, Value: uint64(info.height)},
		},
	}
}

func (c *AtomicCommitor) buildCrtcObject(info *surfaceAtomicInfo) drminternal.AtomicObject {
	props := []drminternal.AtomicProp{
		{ID: info.cProps.active, Value: 1},
	}
	if info.modeBlobID != 0 {
		props = append(props, drminternal.AtomicProp{
			ID:    info.cProps.modeID,
			Value: uint64(info.modeBlobID),
		})
	}
	return drminternal.AtomicObject{
		ID:    info.crtcID,
		Props: props,
	}
}

func (c *AtomicCommitor) buildConnectorObject(info *surfaceAtomicInfo) drminternal.AtomicObject {
	return drminternal.AtomicObject{
		ID: info.connectorID,
		Props: []drminternal.AtomicProp{
			{ID: info.aConn.crtcID, Value: uint64(info.crtcID)},
		},
	}
}

func (c *AtomicCommitor) CommitSingle(info *surfaceAtomicInfo, fbID uint32, modeset bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var objects []drminternal.AtomicObject

	if modeset || !info.modesetDone {
		objects = append(objects, c.buildCrtcObject(info))
		objects = append(objects, c.buildConnectorObject(info))
		objects = append(objects, c.buildPlaneObject(info, fbID))

		flags := drminternal.AtomicFlagPageFlipEvent | drminternal.AtomicFlagAllowModeset
		if err := drminternal.AtomicCommit(c.fd, flags, objects, 0); err != nil {
			return fmt.Errorf("atomic modeset commit: %w", err)
		}
		info.modesetDone = true
	} else {
		objects = append(objects, c.buildPlaneObject(info, fbID))

		flags := drminternal.AtomicFlagPageFlipEvent | drminternal.AtomicFlagNonBlock
		if err := drminternal.AtomicCommit(c.fd, flags, objects, 0); err != nil {
			return fmt.Errorf("atomic page flip: %w", err)
		}
	}

	return nil
}

func (c *AtomicCommitor) AddPending(crtcID uint32, fbID uint32, first bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	info := c.infos[crtcID]
	if info == nil {
		return
	}
	c.pending[crtcID] = &pendingFlip{info: info, fbID: fbID, first: first}
}

func (c *AtomicCommitor) Commit() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.pending) == 0 {
		return nil
	}

	var objects []drminternal.AtomicObject
	needsModeset := false

	for _, pf := range c.pending {
		if pf.first || !pf.info.modesetDone {
			needsModeset = true
			objects = append(objects, c.buildCrtcObject(pf.info))
			objects = append(objects, c.buildConnectorObject(pf.info))
		}
		objects = append(objects, c.buildPlaneObject(pf.info, pf.fbID))
	}

	flags := drminternal.AtomicFlagPageFlipEvent | drminternal.AtomicFlagNonBlock
	if needsModeset {
		flags |= drminternal.AtomicFlagAllowModeset
	}

	if err := drminternal.AtomicCommit(c.fd, flags, objects, 0); err != nil {
		return fmt.Errorf("atomic commit: %w", err)
	}

	for _, pf := range c.pending {
		pf.info.modesetDone = true
	}
	c.pending = make(map[uint32]*pendingFlip)

	return nil
}

func (c *AtomicCommitor) Cancel(crtcID uint32) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.pending, crtcID)
}

func (c *AtomicCommitor) GetInfo(crtcID uint32) *surfaceAtomicInfo {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.infos[crtcID]
}

func (c *AtomicCommitor) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, info := range c.infos {
		if info.modeBlobID != 0 {
			drminternal.DestroyBlob(c.fd, info.modeBlobID)
			info.modeBlobID = 0
		}
	}
}

func (c *AtomicCommitor) FD() int { return c.fd }

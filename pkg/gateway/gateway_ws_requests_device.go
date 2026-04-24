package gateway

import (
	"context"
	"time"
)

func (c *openClawWSConn) handleDeviceWSRequest(_ context.Context, frame openClawWSFrame, method string) (bool, error) {
	switch method {
	case "device.pairing.generate":
		if err := c.ensureDevicePairing(frame); err != nil {
			return true, err
		}
		deviceName := mapString(frame.Params, "device_name")
		deviceType := mapString(frame.Params, "device_type")
		if deviceType == "" {
			deviceType = "cli"
		}
		code, err := c.server.devicePairing.GeneratePairingCode(deviceName, deviceType)
		if err != nil {
			return true, c.writeResponse(frame.ID, false, nil, err.Error())
		}
		return true, c.writeResponse(frame.ID, true, map[string]any{
			"code":    code.Code,
			"expires": code.ExpiresAt.Format(time.RFC3339),
			"device":  code.DeviceName,
			"type":    code.DeviceType,
		}, "")
	case "device.pairing.validate":
		if err := c.ensureDevicePairing(frame); err != nil {
			return true, err
		}
		code := mapString(frame.Params, "code")
		if code == "" {
			return true, c.writeResponse(frame.ID, false, nil, "code is required")
		}
		codeObj, err := c.server.devicePairing.ValidatePairingCode(code)
		if err != nil {
			return true, c.writeResponse(frame.ID, false, nil, err.Error())
		}
		return true, c.writeResponse(frame.ID, true, map[string]any{
			"valid":       true,
			"device_name": codeObj.DeviceName,
			"device_type": codeObj.DeviceType,
			"expires":     codeObj.ExpiresAt.Format(time.RFC3339),
		}, "")
	case "device.pairing.pair":
		if err := c.ensureDevicePairing(frame); err != nil {
			return true, err
		}
		code := mapString(frame.Params, "code")
		deviceID := mapString(frame.Params, "device_id")
		deviceName := mapString(frame.Params, "device_name")
		if code == "" || deviceID == "" {
			return true, c.writeResponse(frame.ID, false, nil, "code and device_id are required")
		}
		pairing, err := c.server.devicePairing.CompletePairing(code, deviceID)
		if err != nil {
			return true, c.writeResponse(frame.ID, false, nil, err.Error())
		}
		if deviceName != "" {
			pairing.DeviceName = deviceName
		}
		return true, c.writeResponse(frame.ID, true, pairing, "")
	case "device.pairing.unpair":
		if err := c.ensureDevicePairing(frame); err != nil {
			return true, err
		}
		deviceID := mapString(frame.Params, "device_id")
		if deviceID == "" {
			return true, c.writeResponse(frame.ID, false, nil, "device_id is required")
		}
		if err := c.server.devicePairing.Unpair(deviceID); err != nil {
			return true, c.writeResponse(frame.ID, false, nil, err.Error())
		}
		return true, c.writeResponse(frame.ID, true, map[string]any{"ok": true}, "")
	case "device.pairing.list":
		if err := c.ensureDevicePairing(frame); err != nil {
			return true, err
		}
		devices := c.server.devicePairing.ListPaired()
		return true, c.writeResponse(frame.ID, true, map[string]any{"devices": devices}, "")
	case "device.pairing.status":
		if err := c.ensureDevicePairing(frame); err != nil {
			return true, err
		}
		return true, c.writeResponse(frame.ID, true, c.server.devicePairing.GetStatus(), "")
	case "device.pairing.renew":
		if err := c.ensureDevicePairing(frame); err != nil {
			return true, err
		}
		deviceID := mapString(frame.Params, "device_id")
		if deviceID == "" {
			return true, c.writeResponse(frame.ID, false, nil, "device_id is required")
		}
		pairing, err := c.server.devicePairing.RenewPairing(deviceID)
		if err != nil {
			return true, c.writeResponse(frame.ID, false, nil, err.Error())
		}
		return true, c.writeResponse(frame.ID, true, pairing, "")
	default:
		return false, nil
	}
}

func (c *openClawWSConn) ensureDevicePairing(frame openClawWSFrame) error {
	if c.server.devicePairing == nil {
		return c.writeResponse(frame.ID, false, nil, "device pairing not initialized")
	}
	return nil
}

package machines

import (
	"fmt"

	"internal-developer-platform/src/features/image-security/internal/contracts"
	"internal-developer-platform/src/shared/errors"
)

type ImageScanEvent string

const (
	EventStartScan    ImageScanEvent = "START_SCAN"
	EventScanPass     ImageScanEvent = "SCAN_PASS"
	EventScanFail     ImageScanEvent = "SCAN_FAIL"
	EventGrantWaiver  ImageScanEvent = "GRANT_WAIVER"
	EventRevokeWaiver ImageScanEvent = "REVOKE_WAIVER"
)

func TransitionScanState(current contracts.ScanStatus, event ImageScanEvent) (contracts.ScanStatus, error) {
	switch current {
	case contracts.ScanStatusPending:
		switch event {
		case EventStartScan:
			return contracts.ScanStatusScanning, nil
		}
	case contracts.ScanStatusScanning:
		switch event {
		case EventScanPass:
			return contracts.ScanStatusPassed, nil
		case EventScanFail:
			return contracts.ScanStatusBlocked, nil
		case EventGrantWaiver:
			return contracts.ScanStatusException, nil
		}
	case contracts.ScanStatusBlocked:
		switch event {
		case EventGrantWaiver:
			return contracts.ScanStatusException, nil
		case EventStartScan:
			return contracts.ScanStatusScanning, nil
		}
	case contracts.ScanStatusException:
		switch event {
		case EventRevokeWaiver:
			return contracts.ScanStatusBlocked, nil
		case EventStartScan:
			return contracts.ScanStatusScanning, nil
		}
	case contracts.ScanStatusPassed:
		switch event {
		case EventStartScan:
			return contracts.ScanStatusScanning, nil
		}
	}
	return current, errors.NewConflictError(fmt.Sprintf("illegal scan state transition from %s via %s", current, event))
}

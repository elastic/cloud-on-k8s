package lvm

import (
	"fmt"

	"github.com/elastic/stack-operators/local-volume/pkg/driver/daemon/cmdutil"
)

// ThinPoolLayout represents the layout
const ThinPoolLayout = "thin,pool"

// ThinPool represents an LVM thin pool logical volume
type ThinPool struct {
	LogicalVolume // a ThinPool is a LogicalVolume
	dataPercent   float64
}

// CreateThinVolume creates a thin logical volume
func (tp ThinPool) CreateThinVolume(newCmd cmdutil.FactoryFunc, name string, virtualSizeInBytes uint64) (LogicalVolume, error) {
	if err := ValidateLogicalVolumeName(name); err != nil {
		return LogicalVolume{}, err
	}

	// size must be a multiple of 512
	roundedSize := roundUpTo512(virtualSizeInBytes)

	cmd := newCmd(
		"lvcreate",
		"--virtualsize", fmt.Sprintf("%db", roundedSize),
		"--name", name,
		"--thin",
		"--thinpool", tp.name,
		tp.vg.name,
	)

	if err := RunLVMCmd(cmd, nil); err != nil {
		return LogicalVolume{}, err
	}

	return LogicalVolume{name, roundedSize, tp.vg}, nil
}

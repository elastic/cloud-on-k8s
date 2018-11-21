package lvm

const DriverKind = "LVM"

type Driver struct{}

func NewDriver() *Driver {
	return &Driver{}
}

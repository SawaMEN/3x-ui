// Package sidecaregress defines the stable loopback contract between managed
// protocol servers and the core router. Failure to reach it must fail closed.
package sidecaregress

import "fmt"

const BasePort = 60000
const Slots = 1000

func Port(id int) int          { return BasePort + 2*((id-1)%Slots) }
func Address(id int) string    { return fmt.Sprintf("127.0.0.1:%d", Port(id)) }
func UDPAddress(id int) string { return fmt.Sprintf("127.0.0.1:%d", Port(id)+1) }

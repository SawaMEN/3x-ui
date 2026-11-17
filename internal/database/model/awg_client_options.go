package model

// AWGClientOptions only overrides client-side packet generation and local MTU.
// Nil fields inherit the inbound defaults; an empty object clears overrides.
// Shared header/padding/key parameters deliberately cannot be changed here.
type AWGClientOptions struct {
	Jc   *int    `json:"jc,omitempty"`
	Jmin *int    `json:"jmin,omitempty"`
	Jmax *int    `json:"jmax,omitempty"`
	I1   *string `json:"i1,omitempty"`
	I2   *string `json:"i2,omitempty"`
	I3   *string `json:"i3,omitempty"`
	I4   *string `json:"i4,omitempty"`
	I5   *string `json:"i5,omitempty"`
	MTU  *int    `json:"mtu,omitempty"`
}

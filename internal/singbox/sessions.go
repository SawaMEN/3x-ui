package singbox

// ActiveSession is the stable panel-facing view of a live sing-box connection.
type ActiveSession struct {
	ID          string   `json:"id"`
	Core        string   `json:"core"`
	Inbound     string   `json:"inbound"`
	InboundType string   `json:"inboundType,omitempty"`
	User        string   `json:"user,omitempty"`
	Outbound    string   `json:"outbound,omitempty"`
	Network     string   `json:"network,omitempty"`
	Source      string   `json:"source,omitempty"`
	Destination string   `json:"destination,omitempty"`
	Domain      string   `json:"domain,omitempty"`
	Rule        string   `json:"rule,omitempty"`
	CreatedAt   int64    `json:"createdAt,omitempty"`
	Upload      int64    `json:"upload"`
	Download    int64    `json:"download"`
	Chain       []string `json:"chain,omitempty"`
}

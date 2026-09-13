package gh

// Gateway answers the usecase layer's ports by way of one GitHub client.
type Gateway struct{ backend }

// New wires a gateway to a client.
func New(b backend) *Gateway { return &Gateway{backend: b} }

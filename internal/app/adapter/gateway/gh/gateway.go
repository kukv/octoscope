package gh

type Gateway struct{ backend }

func New(b backend) *Gateway { return &Gateway{backend: b} }

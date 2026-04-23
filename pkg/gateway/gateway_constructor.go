package gateway

// New creates a gateway server shell around the supplied runtime object.
//
// The argument is intentionally accepted as any in this compatibility layer:
// full runtime wiring can pass its concrete MainRuntime without forcing this
// small dependency PR to import the complete runtime package.
func New(mainRuntime any) *Server {
	return &Server{
		mainRuntime: mainRuntime,
		addr:        defaultGatewayAddress,
	}
}

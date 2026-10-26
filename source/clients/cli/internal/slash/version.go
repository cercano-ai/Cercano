package slash

func RegisterVersion(r *Registry) {
	r.Register(Command{Name: "version", Help: "Show running TUI and connected development agent host identities.", Handler: func([]string) Result { return Result{Kind: ResultOpenVersion} }})
}

package main

import "runner/server/app"

func main() {
	app.Main(app.Options{
		ProgramName:       "runner-server",
		DefaultConfigPath: "runner_server.yaml",
	})
}

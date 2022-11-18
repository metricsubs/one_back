package main

import (
	"github.com/pocketbase/pocketbase"
	"log"
	"one_back/services"
)

func main() {
	app := pocketbase.New()

	services.RegisterCustomServices(app)

	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}

package services

import (
	"github.com/pocketbase/pocketbase"
)

func RegisterCustomServices(app *pocketbase.PocketBase) {
	registerS3Service(app)
}

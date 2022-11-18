package services

import "github.com/pocketbase/pocketbase"

func GetInDBConfig(app *pocketbase.PocketBase, configName string) (string, error) {
	record, err := app.Dao().FindFirstRecordByData("configs", "key", configName)
	if err != nil {
		return "", err
	}
	return record.GetString("value"), nil
}

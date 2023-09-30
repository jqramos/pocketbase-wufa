package main

import (
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"wufa-app/wufa_api"
	loan_service "wufa-app/wufa_core"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

func main() {
	app := pocketbase.New()
	zerolog.SetGlobalLevel(zerolog.DebugLevel)

	app.OnBeforeServe().Add(func(e *core.ServeEvent) error {
		wufa_api.BindPaymentApiRoutes(app, e.Router.Group("/internal"))
		return nil
	})

	app.OnModelAfterCreate("loans").Add(func(e *core.ModelEvent) error {
		log.Print(e.Model.TableName())
		log.Print(e.Model.GetId())
		//call loan service
		err := loan_service.TriggerOnCreateLoanSchedule(e.Model.GetId(), app)
		if err != nil {
			return err
		}
		return nil
	})

	if err := app.Start(); err != nil {
		log.Error().Err(err).Msg("Failed to start app")
	}

}

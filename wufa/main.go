package main

import (
	"log"
	"wufa-app/wufa_api"
	loan_service "wufa-app/wufa_core"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

func main() {
	app := pocketbase.New()

	app.OnBeforeServe().Add(func(e *core.ServeEvent) error {
		log.Println("Binding custom api routes")

		wufa_api.BindPaymentApiRoutes(app, e.Router.Group("/internal"))

		return nil
	})

	app.OnModelAfterCreate("loans").Add(func(e *core.ModelEvent) error {
		log.Println(e.Model.TableName())
		log.Println(e.Model.GetId())
		//call loan service
		err := loan_service.TriggerOnCreateLoanSchedule(e.Model.GetId(), app)
		if err != nil {
			return err
		}
		return nil
	})

	if err := app.Start(); err != nil {
		log.Fatal(err)
	}

}

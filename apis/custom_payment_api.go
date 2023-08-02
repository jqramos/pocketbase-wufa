package apis

import (
	"log"
	"net/http"

	"github.com/labstack/echo/v5"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/models"
)

const loanCollectionNameOrId = "loans"
const investorCollectionNameOrId = "investors"
const transactionsCollectionNameOrId = "transactions"

func PaymentApiRoutes(app core.App, rg *echo.Group) {
	api := paymentRecordApi{app: app}

	subGroup := rg.Group(
		"/payment",
		ActivityLogger(app),
		// LoadCollectionContext(app),
	)

	subGroup.POST("", api.cErr)
	subGroup.POST("pay/:id", api.createLoanSchedule, RequireSameContextRecordAuth())
}

type paymentRecordApi struct {
	app core.App
}

func (api *paymentRecordApi) cErr(c echo.Context) error {
	return NewNotFoundError("Error message 1", "Custom: Missing Id")
}

func (api *paymentRecordApi) createLoanSchedule(c echo.Context) error {

	recordId := c.PathParam("id")
	if recordId == "" {
		return NewNotFoundError("Error message 2", "Custom: Missing Id")
	}

	// recordId = recordId[4: len(recordId)-4]
	//get record from loan collection by id and get customerId and investor
	record, fetchErr := api.app.Dao().FindRecordById(loanCollectionNameOrId, recordId, nil)

	if fetchErr != nil || record == nil {
		// return NewNotFoundError("Not Found..", fetchErr)
		return NewNotFoundError("", fetchErr)
	}

	//call subtractLoanFromInvestorBalance
	subtractLoanFromInvestorBalance(c, api.app, record)

	if fetchErr != nil || record == nil {
		// return NewNotFoundError("Not Found..", fetchErr)
		return NewNotFoundError("", fetchErr)
	}

	if fetchErr != nil || record == nil {
		// return NewNotFoundError("Not Found..", fetchErr)
		return NewNotFoundError("", fetchErr)
	}

	event := &core.RecordViewEvent{
		HttpContext: c,
		Record:      record,
	}

	if fetchErr != nil || record == nil {
		// return NewNotFoundError("Not Found..", fetchErr)
		return NewNotFoundError("", fetchErr)
	}

	return api.app.OnRecordViewRequest().Trigger(event, func(e *core.RecordViewEvent) error {
		if err := EnrichRecord(e.HttpContext, api.app.Dao(), e.Record); err != nil && api.app.IsDebug() {
			log.Println(err)
		}

		result := map[string]string{
			"name":  e.Record.GetString("name"),
			"state": e.Record.GetString("state"),
		}
		// return e.HttpContext.JSON(http.StatusOK, e.Record)
		return e.HttpContext.JSON(http.StatusOK, result)
	})
}

// func subtractLoanFromInvestorBalance
func subtractLoanFromInvestorBalance(c echo.Context, app core.App, record *models.Record) error {

	//get investor record
	investorId := record.ExpandedOne("investor")
	investorRecord, fetchErr := app.Dao().FindRecordById(investorCollectionNameOrId, investorId.GetString("id"), nil)

	if fetchErr != nil || investorRecord == nil {
		// return NewNotFoundError("Not Found..", fetchErr)
		return NewNotFoundError("", fetchErr)
	}

	//get investor balance
	investorBalance := investorRecord.GetFloat("investmentBalance")

	//get loan amount
	loanAmount := record.GetFloat("amount")

	//subtract loan amount from investor balance
	investorBalance = investorBalance - loanAmount

	transactionsCollection, err := app.Dao().FindCollectionByNameOrId(transactionsCollectionNameOrId)
	if err != nil {
		return err
	}

	//create transaction record
	transactionRecord := models.NewRecord(transactionsCollection)
	transactionRecord.Set("type", "debit")
	transactionRecord.Set("amount", loanAmount)
	transactionRecord.Set("description", "Loan to customer"+record.GetString("customer.name"))

	//update investor balance
	investorRecord.Set("balance", investorBalance)

	//save investor record
	// saveErr := app.Dao().SaveRecord(investorCollectionNameOrId, investorRecord)

	// if saveErr != nil {
	// 	return NewNotFoundError("", saveErr)
	// }

	return nil
}

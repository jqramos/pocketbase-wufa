package apis

import (
	"github.com/labstack/echo/v5"
	"github.com/pocketbase/pocketbase/core"
)

const loanCollectionNameOrId = "loans"
const investorCollectionNameOrId = "investors"
const transactionsCollectionNameOrId = "transactions"

// For loan transaction like payment and disbursement
func PaymentApiRoutes(app core.App, rg *echo.Group) {
	api := paymentRecordApi{app: app}

	subGroup := rg.Group(
		"/payment",
		ActivityLogger(app),
		// LoadCollectionContext(app),
	)

	subGroup.POST("", api.cErr)
	subGroup.POST("pay/:id", api.markAsPaid, RequireSameContextRecordAuth())
}

type paymentRecordApi struct {
	app core.App
}

func (api *paymentRecordApi) cErr(c echo.Context) error {
	return NewNotFoundError("Error message 1", "Custom: Missing Id")
}

func (api *paymentRecordApi) markAsPaid(c echo.Context) error {
	loanId := c.PathParam("id")
	//get loan record
	loanRecord, err := api.app.Dao().FindRecordById(loanCollectionNameOrId, loanId, nil)
	if err != nil {
		return NewNotFoundError("Error message 2", "Custom: Loan record not found")
	}

}

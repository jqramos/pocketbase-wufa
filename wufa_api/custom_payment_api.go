package wufa_api

import (
	"log"
    "encoding/csv"
	loan_service "myapp/wufa_core"

	"github.com/labstack/echo/v5"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

const loanCollectionNameOrId = "loans"
const investorCollectionNameOrId = "investors"
const transactionsCollectionNameOrId = "transactions"

const (
	TransactionStatusPending string = "PENDING"
	//Debit
	TrasactionStatusDebit = "DEBIT"
	//Credit
	TransactionStatusCredit = "CREDIT"
)

const (
	LoanStatusOngoing string = "Ongoing"
	//Unpaid
	LoanStatusUnpaid = "Unpaid"
	//Completed
	LoanStatusCompleted = "Completed"
)

// For loan transaction like payment and disbursement
func BindPaymentApiRoutes(app core.App, rg *echo.Group) {
	api := paymentRecordApi{app: app}
	log.Println("Binding payment api routes")
	subGroup := rg.Group(
		"/payment",
		apis.ActivityLogger(app),
		apis.RequireRecordAuth(),
	)

	subGroup.POST("/pay/:id", api.markAsPaid)
	subGroup.POST("/batch-file", api.batchFileProcess)
}

type paymentRecordApi struct {
	app core.App
}

func (api *paymentRecordApi) cErr(c echo.Context) error {
	return apis.NewNotFoundError("Error message 1", "Custom: Missing Id")
}



func (api *paymentRecordApi) markAsPaid(c echo.Context) error {
	data := apis.RequestInfo(c).Data

	loanId := c.PathParam("id")
	transactionId := data["transactionId"].(string)
	transactionDate := data["transactionDate"].(string)
	log.Println("loanId", loanId)

	//check loanId
	if loanId == "" {
		return apis.NewNotFoundError("Error message 1", "Custom: Missing Id")
	}

	//get loan record
	loanRecord, err := api.app.Dao().FindRecordById(loanCollectionNameOrId, loanId, nil)
	if err != nil {
		log.Fatalf("failed to expand: %v", err)
		return apis.NewNotFoundError("Error message 2", "Custom: Loan record not found")
	}

	if errs := api.app.Dao().ExpandRecord(loanRecord, []string{"investor"}, nil); len(errs) > 0 {
		log.Fatalf("failed to expand: %v", err)
		return apis.NewNotFoundError("Error message 2", "Custom: Loan record not found")
	}

	//get transaction record
	transactionRecord, err := api.app.Dao().FindRecordById(transactionsCollectionNameOrId, transactionId , nil)
	if err != nil {
		log.Fatalf("failed to expand: %v", err)
		return apis.NewNotFoundError("Error message 3", "Custom: Transaction record not found")
	}

	//get loan status
	loanStatus := loanRecord.GetString("status")
	//proceed only if status is ongoing
	if loanStatus != LoanStatusOngoing {
		log.Fatalf("failed to expand: %v", err)
		return apis.NewNotFoundError("Error message 4", "Custom: Loan status is not ongoing")
	}

	//get loan amount
	loanAmount := loanRecord.GetFloat("remainingBalance")
	transactionAmount := transactionRecord.GetFloat("amount")
	//subtract loan amount from transaction amount
	var remainingBalance = loanAmount - transactionAmount

	//get paid amount and add transaction amount
	paidAmount := loanRecord.GetFloat("paidAmount")
	paidAmount = paidAmount + transactionAmount

	//update loan record
	loanRecord.Set("remainingBalance", remainingBalance)
	loanRecord.Set("paidAmount", paidAmount)

	//if remainingBalance is 0, update loan status to completed
	if remainingBalance == 0 {
		loanRecord.Set("status", LoanStatusCompleted)
	}

	//save loan record
	if err := api.app.Dao().SaveRecord(loanRecord); err != nil {
		log.Fatalf("failed to expand: %v", err)
		return apis.NewNotFoundError("Error message 4", "Custom: Failed to save loan record")
	}

	//update transaction record status to credit
	transactionRecord.Set("type", TransactionStatusCredit)
	transactionRecord.Set("transactionDate", transactionDate)

	//save transaction record
	if err := api.app.Dao().SaveRecord(transactionRecord); err != nil {
		log.Fatalf("failed to expand: %v", err)
		return apis.NewNotFoundError("Error message 5", "Custom: Failed to save transaction record")
	}

	//get investor record from loanRecord
	investorRecord := loanRecord.ExpandedOne("investor")
	//get investor balance
	investorBalance := investorRecord.GetFloat("investmentBalance")
	//alter investor balance
	investorBalance = investorBalance + transactionRecord.GetFloat("amount")
	//get loaned amount
	loanedAmount := investorRecord.GetFloat("loanedAmount")
	//subtract loaned amount from transaction amount
	var newLoanedAmount = loanedAmount - transactionAmount
	investorRecord.Set("investmentBalance", investorBalance)
	investorRecord.Set("loanedAmount", newLoanedAmount)

	//save investor record
	if err := api.app.Dao().SaveRecord(investorRecord); err != nil {
		log.Fatalf("failed to expand: %v", err)
		return apis.NewNotFoundError("Error message 6", "Custom: Failed to save investor record")
	}

	//return success
	return c.JSON(200, map[string]any{"message": "Success"})

}

func (api *paymentRecordApi) batchFileProcess(c echo.Context) error{

	//get file from request
	file, err := c.FormFile("file")
	if err != nil {
		log.Fatalf("failed to expand: %v", err)
		return apis.NewNotFoundError("Error message 1", "Custom: Missing file")
	}
	//check if file is csv
	if file.Header.Get("Content-Type") != "text/csv" {
		log.Fatalf("failed to expand: %v", err)
		return apis.NewNotFoundError("Error message 2", "Custom: Invalid file type")
	}

	//open file
	src, err := file.Open()
	if err != nil {
		log.Fatalf("failed to expand: %v", err)
		return apis.NewNotFoundError("Error message 3", "Custom: Failed to open file")
	}

	//read file
	reader := csv.NewReader(src)

	//get all records
	records, err := reader.ReadAll()
	if err != nil {
		log.Fatalf("failed to expand: %v", err)
		return apis.NewNotFoundError("Error message 4", "Custom: Failed to read file")
	}

	//pass records to loan_service
	result, err := loan_service.LoadCSVFileToData(records)

	//return result
	return c.JSON(200, map[string]any{"message": "Success", "result": result})
	

}
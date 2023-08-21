package loan_service

import (
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/models"
)

const loanCollectionNameOrId = "loans"
const investorCollectionNameOrId = "investors"
const transactionsCollectionNameOrId = "transactions"

func TriggerOnCreateLoanSchedule(loanId string, app core.App) error {
	loan, err := app.Dao().FindRecordById(loanCollectionNameOrId, loanId, nil)

	if errs := app.Dao().ExpandRecord(loan, []string{"customerId", "investor"}, nil); len(errs) > 0 {
		log.Fatalf("failed to expand: %v", errs)
		return fmt.Errorf("failed to expand: %v", errs)
	}
	//get investor record
	investorRecord := loan.ExpandedOne("investor")
	customer := loan.ExpandedOne("customerId")

	createPayments(loan, investorRecord.GetString("id"), app, customer.GetString("customerName"))
	investorRecord, fetchErr := app.Dao().FindRecordById(investorCollectionNameOrId, investorRecord.GetString("id"), nil)

	if fetchErr != nil || investorRecord == nil {
		log.Fatal(fetchErr)
	}

	//get investor balance
	investorBalance := investorRecord.GetFloat("investmentBalance")

	//get loan amount
	loanAmount := loan.GetFloat("amount")
	var loanedAmount = investorRecord.GetFloat("loanedAmount")

	//subtract loan amount from investor balance
	investorBalance = investorBalance - loanAmount
	var newLoanedAmount = loanedAmount + loanAmount

	//update investor balance
	investorRecord.Set("investmentBalance", investorBalance)
	investorRecord.Set("loanedAmount", newLoanedAmount)
	//save investor record
	if err := app.Dao().SaveRecord(investorRecord); err != nil {
		return err
	}

	transactionsCollection, err := app.Dao().FindCollectionByNameOrId(transactionsCollectionNameOrId)
	if err != nil {
		return err
	}

	//create transaction record
	transactionRecord := models.NewRecord(transactionsCollection)
	transactionRecord.Set("investor", investorRecord.GetString("id"))
	transactionRecord.Set("type", "DEBIT")
	transactionRecord.Set("amount", loanAmount)
	transactionRecord.Set("cashBalance", investorBalance)
	transactionRecord.Set("description", "Loan to customer "+loan.GetString("customerName"))
	transactionRecord.Set("transactionDate", loan.GetDateTime("startDate"))

	if err := app.Dao().SaveRecord(transactionRecord); err != nil {
		return err
	}

	return nil
}

func createPayments(loan *models.Record, investorId string, app core.App, customerName string) {
	startDate := loan.GetDateTime("startDate")
	amount := loan.GetFloat("amount") * 1.2
	var dates = getDates(startDate)

	transactionsCollection, err := app.Dao().FindCollectionByNameOrId(transactionsCollectionNameOrId)
	if err != nil {
		log.Fatal(err)
	}

	//divide amount by 8
	var paymentPerWeek = amount / float64(8)
	//create payment records based on number of dates
	for i := 0; i < len(dates); i++ {
		//create payment record
		var weekNumber = i + 1
		paymentRecord := models.NewRecord(transactionsCollection)
		paymentRecord.Set("customerName", customerName)
		paymentRecord.Set("amount", paymentPerWeek)
		paymentRecord.Set("targetDate", dates[i])
		paymentRecord.Set("type", "PENDING")
		paymentRecord.Set("loan", loan.GetString("id"))
		paymentRecord.Set("investor", investorId)
		paymentRecord.Set("description", "Payment for week "+strconv.Itoa(weekNumber))
		//save payment record
		saveErr := app.Dao().SaveRecord(paymentRecord)
		if saveErr != nil {
			log.Fatal(saveErr)
		}
	}

}

func getDates(startDate types.DateTime) []types.DateTime {
	var dates []types.DateTime
	//add oneday to startDate
	startDate, err := types.ParseDateTime(startDate)
	if err != nil {
		log.Fatal(err)
	}
	//1 day and 8 hours
	var trueStart = startDate.Time().AddDate(0, 0, 1)
	trueStart = trueStart.Add(time.Hour * 8)
	//increment one  week 8 times
	for i := 0; i < 8; i++ {
		//add one week to startDate
		trueStart = trueStart.AddDate(0, 0, 7)
		convDate, err := types.ParseDateTime(trueStart)
		if err != nil {
			log.Fatal(err)
		}
		log.Println(trueStart.Format("2006-01-02 08:00:00"))
		//append to dates
		dates = append(dates, convDate)
	}

	return dates
}

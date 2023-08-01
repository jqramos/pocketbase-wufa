package main

import (
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/models"
	"github.com/pocketbase/pocketbase/tools/types"
)

const loanCollectionNameOrId = "loans"
const investorCollectionNameOrId = "investors"
const transactionsCollectionNameOrId = "transactions"

func main() {
	app := pocketbase.New()
	// serves static files from the provided public dir (if exists)
	app.OnBeforeServe().Add(func(e *core.ServeEvent) error {
		e.Router.GET("/*", apis.StaticDirectoryHandler(os.DirFS("./pb_public"), false))
		return nil
	})

	app.OnModelAfterCreate("loans").Add(func(e *core.ModelEvent) error {
		log.Println(e.Model.TableName())
		log.Println(e.Model.GetId())
		//call loan service
		triggerOnCreateLoanSchedule(e.Model.GetId(), app)
		return nil
	})

	if err := app.Start(); err != nil {
		log.Fatal(err)
	}

}

func triggerOnCreateLoanSchedule(loanId string, app core.App) error {
	loan, err := app.Dao().FindRecordById(loanCollectionNameOrId, loanId, nil)

	if errs := app.Dao().ExpandRecord(loan, []string{"customerId", "investor"}, nil); len(errs) > 0 {
		log.Fatalf("failed to expand: %v", errs)
		return fmt.Errorf("failed to expand: %v", errs)
	}
	//get investor record
	investorRecord := loan.ExpandedOne("investor")
	log.Println("investor record")
	createPayments(loan, investorRecord.GetString("id"), app)
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
	transactionRecord.Set("description", "Loan to customer "+loan.GetString("customer.name"))
	transactionRecord.Set("transactionDate", types.NowDateTime())

	if err := app.Dao().SaveRecord(transactionRecord); err != nil {
		return err
	}

	return nil
}

func createPayments(loan *models.Record, investorId string, app core.App) {
	customer := loan.ExpandedOne("customerId")
	startDate := loan.GetDateTime("startDate")
	amount := loan.GetFloat("amount")
	var dates = getDates(startDate)

	transactionsCollection, err := app.Dao().FindCollectionByNameOrId(transactionsCollectionNameOrId)
	if err != nil {
		log.Fatal(err)
	}

	//divide amount by 8
	var paymentPerWeek = amount / 8
	//create payment records based on number of dates
	for i := 0; i < len(dates); i++ {
		//create payment record
		var customerName = customer.GetString("customerName")
		var weekNumber = i + 1
		paymentRecord := models.NewRecord(transactionsCollection)
		paymentRecord.Set("customerId", customerName)
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
	var trueStart = startDate.Time().AddDate(0, 0, 1)
	//increment one  week 8 times
	for i := 0; i < 8; i++ {
		//add one week to startDate
		trueStart = trueStart.AddDate(0, 0, 7)
		convDate, err := types.ParseDateTime(trueStart)
		if err != nil {
			log.Fatal(err)
		}

		log.Println(trueStart.Format("2006-01-02 15:04:05"))
		//append to dates
		dates = append(dates, convDate)
	}

	return dates
}

package loan_service

import (
	"fmt"
	"github.com/rs/zerolog/log"
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
		log.Error().Err(err).Msg("failed to expand")
		return fmt.Errorf("failed to expand: %v", errs)
	}

	investorRecord := loan.ExpandedOne("investor")
	customer := loan.ExpandedOne("customerId")

	createPayments(loan, investorRecord.GetString("id"), app, customer.GetString("customerName"))
	investorRecord, fetchErr := app.Dao().FindRecordById(investorCollectionNameOrId, investorRecord.GetString("id"), nil)

	if fetchErr != nil || investorRecord == nil {
		log.Error().Err(err).Msg("failed to expand")
	}

	investorBalance := investorRecord.GetFloat("investmentBalance")

	loanAmount := loan.GetFloat("amount")
	var loanedAmount = investorRecord.GetFloat("loanedAmount")

	investorBalance = investorBalance - loanAmount
	var newLoanedAmount = loanedAmount + (loanAmount * 1.2)

	investorRecord.Set("investmentBalance", investorBalance)
	investorRecord.Set("loanedAmount", newLoanedAmount)
	log.Debug().Msgf("loanedAmount: %f", newLoanedAmount)
	if err := app.Dao().SaveRecord(investorRecord); err != nil {
		return err
	}

	transactionsCollection, err := app.Dao().FindCollectionByNameOrId(transactionsCollectionNameOrId)
	if err != nil {
		return err
	}

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
		log.Error().Err(err).Msg("failed to find collection")
	}

	var paymentPerWeek = amount / float64(8)

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
			log.Error().Err(saveErr).Msg("failed to save record")
		}
	}

}

func getDates(startDate types.DateTime) []types.DateTime {
	var dates []types.DateTime

	startDate, err := types.ParseDateTime(startDate)
	if err != nil {
		log.Error().Err(err).Msg("failed to parse date")
	}

	var trueStart = startDate.Time()

	if trueStart.Before(time.Date(2023, 4, 28, 0, 0, 0, 0, time.Local)) {
		trueStart = trueStart.AddDate(0, 0, 1)
	}

	for i := 0; i < 8; i++ {

		trueStart = trueStart.AddDate(0, 0, 7)

		currentDate := time.Now()
		hour, min, sec := currentDate.Clock()
		millis := currentDate.Nanosecond() / 1000000
		trueStart = time.Date(trueStart.Year(), trueStart.Month(), trueStart.Day(), hour, min, sec, millis, time.Local)

		convDate, err := types.ParseDateTime(trueStart)
		if err != nil {
			log.Error().Err(err).Msg("failed to parse date")
		}

		dates = append(dates, convDate)
	}

	return dates
}

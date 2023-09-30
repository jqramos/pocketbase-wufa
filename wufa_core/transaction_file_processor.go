package loan_service

import (
	"fmt"
	"github.com/pocketbase/pocketbase/tools/types"
	"github.com/rs/zerolog/log"
	"mime/multipart"
	"strconv"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/models"
	"github.com/xuri/excelize/v2"
)

// enum for cell column
const (
	TransactionDate int = 0
	Amount              = 1
	PaymentType         = 2
	InvestorName        = 3
	CustomerName        = 4
	StartDate           = 5
	TransactionType     = 6
)

const (
	CREDIT   string = "CREDIT"
	DEBIT           = "DEBIT"
	PENDING         = "PENDING"
	DEPOSIT         = "DEPOSIT"
	LOAN            = "LOAN"
	WITHDRAW        = "WITHDRAW"
	PAYMENT         = "PAYMENT"
)

// struct for excel data
type TransData struct {
	TransactionDate  string
	Amount           float64
	PaymentType      string
	InvestorName     string
	CustomerName     string
	IsAdvancePayment bool
	StartDate        string
	TransType        string
	CashBalance      float64
	Description      string
	LoanId           string
}

const customerCollectionNameOrId = "customers"

type processorApp struct {
	app core.App
}

var currentDate string = ""
var secondsToAdd int = 0

// load excel file to data accepts file
func LoadExcelFileToData(file *multipart.FileHeader, app core.App) (string, error) {
	log.Info().Msg("Loading excel file to data")
	//set app to processorApp
	service := processorApp{app: app}
	//convert to io reader
	fileReader, err := file.Open()
	if err != nil {
		log.Debug().Err(err).Msg("failed to open file")
		return "", err
	}

	f, err := excelize.OpenReader(fileReader)
	if err != nil {
		log.Debug().Err(err).Msg("failed to open file")
		return "", err
	}

	// Get all the rows in the Sheet1.
	rows, err := f.GetRows("Sheet1")
	if err != nil {
		log.Debug().Err(err).Msg("failed to get rows")
		return "", err
	}
	for idx, row := range rows {
		if idx != 0 {
			transData := new(TransData)
			log.Debug().Msg("Processing row: " + strconv.Itoa(idx))
			if isStringEmpty(row[TransactionDate]) {
				log.Info().Msg("TransactionDate is empty")
			}
			transData.TransactionDate = row[TransactionDate]
			log.Info().Msg("TransactionDate: " + row[TransactionDate])

			if isStringEmpty(row[Amount]) {
				log.Info().Msg("Amount is empty")
			} else {
				transData.Amount, err = strconv.ParseFloat(row[Amount], 64)
				if err != nil {
					log.Error().Err(err).Msg("failed to parse float")
					return "", err
				}
			}
			log.Info().Msg("Amount: " + row[Amount])

			if isStringEmpty(row[PaymentType]) {
				log.Info().Msg("PaymentType is empty")
			}
			transData.PaymentType = row[PaymentType]
			log.Info().Msg("PaymentType: " + row[PaymentType])

			if isStringEmpty(row[InvestorName]) {
				log.Info().Msg("InvestorName is empty")
			}
			transData.InvestorName = row[InvestorName]

			if isStringEmpty(row[CustomerName]) {
				log.Info().Msg("CustomerName is empty")
			}
			transData.CustomerName = row[CustomerName]

			if isStringEmpty(row[StartDate]) {
				log.Info().Msg("StartDate is empty")
			}

			transData.StartDate = row[StartDate]

			if isStringEmpty(row[TransactionType]) {
				log.Info().Msg("TransactionType is empty")
			}
			transData.TransType = row[TransactionType]

			_, err := service.runDataLoadProcess(*transData)
			if err != nil {
				log.Error().Err(err).Msg("failed to run data load process")
			}

		}
		log.Info().Msg("End of row number: " + strconv.Itoa(idx))
	}
	return "", nil
}

// func to validate string is empty or blank space or empty string
func isStringEmpty(str string) bool {
	if str == "" || str == " " || str == "  " {
		return true
	}
	return false
}

func (service *processorApp) runDataLoadProcess(transData TransData) (string, error) {
	log.Info().Msg("Starting data load process")
	var isNewCustomer = false
	if transData.TransType == LOAN || transData.TransType == PAYMENT {
		//validate customer name
		if transData.CustomerName == "" {
			log.Debug().Msg("Customer name is empty")
			return "Error: Customer name is empty", nil
		}
		customerRecord, err := service.app.Dao().FindFirstRecordByData(customerCollectionNameOrId, "customerName", transData.CustomerName)
		if err != nil {
			log.Debug().Err(err).Msg("failed to find customer record")
			isNewCustomer = true
			log.Info().Msg("Creating new customer record")
			customerRecord = service.createNewCustomer(customerRecord, transData)
		}

		if !isNewCustomer {
			var filter = fmt.Sprintf("customerId = '%s' && status = 'Ongoing'", customerRecord.GetId())
			//get loans
			loans, err := service.app.Dao().FindRecordsByFilter(loanCollectionNameOrId, filter, "", 100, 0, nil)
			if err != nil {
				log.Error().Err(err).Msg("failed to get loans")
				return "Error: Failed to get loans", nil
			}

			//check result
			if len(loans) == 0 && transData.TransType == LOAN {
				log.Debug().Msg("Creating new loan")
				//increment renewalCount by 1
				customerRecord.Set("renewalCount", customerRecord.GetInt("renewalCount")+1)
				//save customer record
				err = service.app.Dao().SaveRecord(customerRecord)
				if err != nil {
					log.Error().Err(err).Msg("failed to save customer record")
					return "Error: Failed to save customer record", nil
				}
				return service.processNewLoan(customerRecord, transData)
			} else {
				log.Info().Msg("Processing existing loan")
				isLastPayment := false
				var balance = loans[0].GetFloat("remainingBalance") - transData.Amount
				if balance <= 0 {
					isLastPayment = true
				}
				return service.processExistingLoan(customerRecord, loans[0], transData, isLastPayment)
			}
		} else {
			return service.processNewLoan(customerRecord, transData)
		}
	} else if transData.TransType == DEPOSIT || transData.TransType == WITHDRAW {
		result, err := service.processInvestorTransaction(transData)
		if err != nil {
			log.Error().Err(err).Msg("failed to process investor transaction")
			return "Error: Failed to process investor transaction", nil
		}
		return result, nil
	}

	return "", nil
}

func (service *processorApp) processExistingLoan(customerRecord *models.Record, loanRecord *models.Record, transData TransData, isLastPayment bool) (string, error) {
	//get investor record
	investorRecord, err := service.app.Dao().FindFirstRecordByData(investorCollectionNameOrId, "investorName", transData.InvestorName)
	if err != nil {
		log.Debug().Err(err).Msg("failed to find investor record")
		return "", err
	}

	//check if debit or credit
	if transData.TransType == PAYMENT {
		//update loan record
		loanRecord.Set("remainingBalance", loanRecord.GetFloat("remainingBalance")-transData.Amount)
		loanRecord.Set("paidAmount", loanRecord.GetFloat("paidAmount")+transData.Amount)
		if isLastPayment {
			log.Info().Msg("Loan is completed")
			loanRecord.Set("status", "Completed")
			var targetDate, _ = service.parseDate(transData.TransactionDate, false)

			loanRecord.Set("endDate", targetDate)
		}

		//save loan record
		err = service.app.Dao().SaveRecord(loanRecord)
		if err != nil {
			log.Error().Err(err).Msg("failed to save loan record")
			return "Error saving loan", err
		}
		transData.CashBalance = investorRecord.GetFloat("investmentBalance") + transData.Amount
		var newLoanedAmount = investorRecord.GetFloat("loanedAmount") - transData.Amount
		//update investor record
		investorRecord.Set("investmentBalance", transData.CashBalance)
		investorRecord.Set("loanedAmount", newLoanedAmount)
		log.Debug().Msgf("Loaned amount: %f", newLoanedAmount)
		//save investor record
		err = service.app.Dao().SaveRecord(investorRecord)
		if err != nil {
			log.Error().Err(err).Msg("failed to save investor record")
			return "", err
		}
		log.Debug().Msgf("Updating investor record %s", investorRecord)

		_, err = service.updateLoanTransaction(transData, loanRecord.GetId(), isLastPayment)
		if err != nil {
			log.Error().Err(err).Msg("failed to update loan transaction")
			return "", err
		}
		return "Success: Transaction processed successfully", nil
	}
	return "", nil

}

func (service *processorApp) processNewLoan(customerRecord *models.Record, transData TransData) (string, error) {
	//get loan collection
	loanCollection, err := service.app.Dao().FindCollectionByNameOrId(loanCollectionNameOrId)
	if err != nil {
		log.Error().Err(err).Msg("failed to get loan collection")
		return "", err
	}
	//get investor record
	investorRecord, err := service.app.Dao().FindFirstRecordByData(investorCollectionNameOrId, "investorName", transData.InvestorName)
	if err != nil {
		log.Error().Err(err).Msg("failed to find investor record")
		return "", err
	}

	var interestRate = 1.2

	//create loan record
	loanRecord := models.NewRecord(loanCollection)
	loanRecord.Set("customerId", customerRecord.GetId())
	loanRecord.Set("loanAmount", transData.Amount)
	loanRecord.Set("amount", transData.Amount)
	loanRecord.Set("status", "Ongoing")
	loanRecord.Set("investor", investorRecord.GetId())
	var startDate, _ = service.parseDate(transData.StartDate, false)
	loanRecord.Set("startDate", startDate)
	loanRecord.Set("renewalCount", 0)
	loanRecord.Set("remainingBalance", transData.Amount*interestRate)
	loanRecord.Set("paidAmount", 0)

	//save
	if err := service.app.Dao().SaveRecord(loanRecord); err != nil {
		log.Error().Err(err).Msg("failed to save loan record")
		return "", err
	}

	return "Success", nil
}

// parse date string to date
func (service *processorApp) parseDate(dateString string, isTransactionDate bool) (types.DateTime, error) {
	date, error := time.Parse("01/02/2006", dateString)
	if error != nil {
		log.Error().Err(error).Msg("failed to parse date")
		return types.DateTime{}, error
	}
	currentDate := time.Now()
	hour, min, sec := currentDate.Clock()
	date = time.Date(date.Year(), date.Month(), date.Day(), hour, min, sec, currentDate.Nanosecond(), time.Local)
	return types.ParseDateTime(date)
}

func (service *processorApp) createNewCustomer(customerRecord *models.Record, transData TransData) *models.Record {
	//get collection
	customerCollection, err := service.app.Dao().FindCollectionByNameOrId(customerCollectionNameOrId)
	if err != nil {
		log.Error().Err(err).Msg("failed to get customer collection")
		return nil
	}
	//create customer record
	customerRecord = models.NewRecord(customerCollection)
	customerRecord.Set("customerName", transData.CustomerName)
	customerRecord.Set("status", "ACTIVE")
	customerRecord.Set("renewalCount", 0)
	//save
	if err := service.app.Dao().SaveRecord(customerRecord); err != nil {
		log.Error().Err(err).Msg("failed to save customer record")
	}
	return customerRecord
}

func (service *processorApp) processInvestorTransaction(transData TransData) (string, error) {
	//get investor record
	investorRecord, err := service.app.Dao().FindFirstRecordByData(investorCollectionNameOrId, "investorName", transData.InvestorName)
	if err != nil {
		log.Error().Err(err).Msg("failed to find investor record")
		investorRecord = service.createNewInvestor(transData)
	}
	//check if debit or credit
	if transData.TransType == WITHDRAW {
		transData.CashBalance = investorRecord.GetFloat("investmentBalance") - transData.Amount
		investorRecord.Set("investmentBalance", transData.CashBalance)
		log.Debug().Msg("processing withdraw")
		investorRecord.Set("investmentPoolAmount", investorRecord.GetFloat("investmentPoolAmount")-transData.Amount)

	} else if transData.TransType == DEPOSIT {
		transData.CashBalance = investorRecord.GetFloat("investmentBalance") + transData.Amount
		investorRecord.Set("investmentBalance", transData.CashBalance)
		log.Info().Msg("processing deposit")
		investorRecord.Set("investmentPoolAmount", transData.CashBalance)
		transData.Description = investorRecord.GetString("investorName") + " deposited " + strconv.FormatFloat(transData.Amount, 'f', 2, 64)
	}
	//save investor record
	err = service.app.Dao().SaveRecord(investorRecord)
	if err != nil {
		log.Error().Err(err).Msg("failed to save investor record")
		return "", err
	}

	//record to transaction
	_, err = service.createNewTransaction(transData, investorRecord.GetId())
	if err != nil {
		log.Error().Err(err).Msg("failed to create new transaction")
		return "", err
	}
	return "Success", nil

}

func (service *processorApp) createNewInvestor(transData TransData) *models.Record {
	//get collection
	investorCollection, err := service.app.Dao().FindCollectionByNameOrId(investorCollectionNameOrId)
	if err != nil {
		log.Error().Err(err).Msg("failed to get investor collection")
		return nil
	}
	//create investor record
	investorRecord := models.NewRecord(investorCollection)
	investorRecord.Set("investorName", transData.InvestorName)
	investorRecord.Set("status", "ACTIVE")
	investorRecord.Set("investmentBalance", 0)
	investorRecord.Set("loanedAmount", 0)
	//save
	if err := service.app.Dao().SaveRecord(investorRecord); err != nil {
		log.Error().Err(err).Msg("failed to save investor record")
	}
	return investorRecord

}

func (service *processorApp) updateLoanTransaction(transData TransData, loanId string, isLastPayment bool) (*models.Record, error) {
	//filter by customer id
	var filter = fmt.Sprintf("loan = '%s' && type = '%s'", loanId, PENDING)
	log.Debug().Msgf("filter: %s", filter)
	//find transaction record with earliest target date
	transactionRecords, err := service.app.Dao().FindRecordsByFilter(transactionsCollectionNameOrId, filter, "+targetDate", 20, 0)
	if err != nil {
		log.Error().Err(err).Msg("failed to find transaction record")
		return nil, err
	}

	transactionCollection, err := service.app.Dao().FindCollectionByNameOrId(transactionsCollectionNameOrId)
	if err != nil {
		log.Error().Err(err).Msg("failed to find transaction collection")
		return nil, err
	}

	if len(transactionRecords) == 0 {
		log.Info().Msg("no pending transaction found creating new transaction")
		existingRecords, err := service.app.Dao().FindRecordsByFilter(transactionsCollectionNameOrId, fmt.Sprintf("loan = '%s'", loanId), "+transactionDate", 20, 0)
		if err != nil {
			log.Error().Err(err).Msg("failed to find transaction record")
			return nil, err
		}

		var weekNumber = len(existingRecords)
		var dateTransacted, _ = service.parseDate(transData.TransactionDate, true)
		newTransactionRecord := models.NewRecord(transactionCollection)
		newTransactionRecord.Set("customerName", transData.CustomerName)
		newTransactionRecord.Set("amount", transData.Amount)
		newTransactionRecord.Set("targetDate", dateTransacted)
		newTransactionRecord.Set("transactionDate", dateTransacted)
		newTransactionRecord.Set("status", "PAID")
		newTransactionRecord.Set("loan", loanId)
		newTransactionRecord.Set("investor", existingRecords[0].GetString("investor"))
		newTransactionRecord.Set("description", "Payment for week "+strconv.Itoa(weekNumber+1))
		newTransactionRecord.Set("cashBalance", transData.CashBalance)
		newTransactionRecord.Set("type", transData.PaymentType)
		log.Debug().Msgf("new transaction record: %v", newTransactionRecord)

		saveErr := service.app.Dao().SaveRecord(newTransactionRecord)
		if saveErr != nil {
			log.Error().Err(err).Msg("failed to save transaction record")
			return nil, saveErr
		}

		return newTransactionRecord, nil

	}
	transData.IsAdvancePayment = len(transactionRecords) > 0 && isLastPayment

	if transData.IsAdvancePayment {
		var cashBalance = transData.CashBalance - transData.Amount
		log.Debug().Msgf("Processing advance payment. Cash balance: %f", cashBalance)
		//update all transaction records
		for _, transactionRecord := range transactionRecords {
			cashBalance = cashBalance + transactionRecord.GetFloat("amount")
			var dateTransacted, _ = service.parseDate(transData.TransactionDate, true)
			transactionRecord.Set("status", "PAID")
			transactionRecord.Set("transactionDate", dateTransacted)
			transactionRecord.Set("type", transData.PaymentType)
			transactionRecord.Set("investorName", transData.InvestorName)
			transactionRecord.Set("customerName", transData.CustomerName)
			transactionRecord.Set("cashBalance", cashBalance)
			transactionRecord.Set("description", "Advance Payment")
			//save
			if err := service.app.Dao().SaveRecord(transactionRecord); err != nil {
				log.Error().Err(err).Msg("failed to save transaction record")
				return nil, err
			}
			log.Debug().Msgf("new transaction record: %v", transactionRecord)
			time.Sleep(5 * time.Millisecond)
		}
		log.Debug().Msgf("Cash balance: %f", cashBalance)
		log.Debug().Msgf("Trans Cash balance: %f", cashBalance)

		if transData.CashBalance != cashBalance {
			log.Debug().Msgf("Creating new transaction for advance payment. Cash balance: %f", cashBalance)
			var dateTransacted, _ = service.parseDate(transData.TransactionDate, true)
			newTransactionRecord := models.NewRecord(transactionCollection)
			newTransactionRecord.Set("customerName", transData.CustomerName)
			newTransactionRecord.Set("amount", transData.CashBalance-cashBalance)
			newTransactionRecord.Set("targetDate", dateTransacted)
			newTransactionRecord.Set("transactionDate", dateTransacted)
			newTransactionRecord.Set("status", "PAID")
			newTransactionRecord.Set("loan", loanId)
			newTransactionRecord.Set("investor", transactionRecords[0].GetString("investor"))
			newTransactionRecord.Set("description", "Advance Payment")
			newTransactionRecord.Set("type", transData.PaymentType)

			saveErr := service.app.Dao().SaveRecord(newTransactionRecord)
			if saveErr != nil {
				log.Error().Err(err).Msg("failed to save transaction record")
				return nil, saveErr
			}
			log.Debug().Msgf("new transaction record: %v", newTransactionRecord)
		}
		return transactionRecords[0], nil
	} else {
		log.Debug().Msg("Processing normal payment")
		var transactionRecord = transactionRecords[0]
		var transactionDate, _ = service.parseDate(transData.TransactionDate, true)
		log.Debug().Msgf("transaction date: %v", transactionDate)
		transactionRecord.Set("transactionDate", transactionDate)
		transactionRecord.Set("type", transData.PaymentType)
		transactionRecord.Set("investorName", transData.InvestorName)
		transactionRecord.Set("customerName", transData.CustomerName)
		transactionRecord.Set("cashBalance", transData.CashBalance)
		transactionRecord.Set("status", "PAID")
		//if amount is not equal to transaction amount, then it is a partial payment
		if transData.Amount != transactionRecord.GetFloat("amount") {
			transactionRecord.Set("amount", transData.Amount)
		}
		//save
		if err := service.app.Dao().SaveRecord(transactionRecord); err != nil {
			log.Error().Err(err).Msg("failed to save transaction record")
			return nil, err
		}
		log.Debug().Msgf("new transaction record: %v", transactionRecord)
		return transactionRecord, nil
	}
}

func (service *processorApp) createNewTransaction(transData TransData, investorId string) (*models.Record, error) {
	//create transaction record
	transactionCollection, err := service.app.Dao().FindCollectionByNameOrId(transactionsCollectionNameOrId)
	if err != nil {
		log.Error().Err(err).Msg("failed to find transaction collection")
		return nil, err
	}

	transactionRecord := models.NewRecord(transactionCollection)
	var targetDate, _ = service.parseDate(transData.TransactionDate, true)
	transactionRecord.Set("transactionDate", targetDate)
	transactionRecord.Set("amount", transData.Amount)
	transactionRecord.Set("type", transData.PaymentType)
	transactionRecord.Set("investor", investorId)
	transactionRecord.Set("customerName", transData.CustomerName)
	transactionRecord.Set("cashBalance", transData.CashBalance)
	transactionRecord.Set("description", transData.Description)
	//save
	if err := service.app.Dao().SaveRecord(transactionRecord); err != nil {
		log.Error().Err(err).Msg("failed to save transaction record")
		return nil, err
	}
	log.Debug().Msgf("new transaction record: %v", transactionRecord)
	return transactionRecord, nil

}

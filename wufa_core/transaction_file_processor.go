package loan_service

import (
	"fmt"
	"github.com/pocketbase/pocketbase/tools/types"
	"log"
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
	log.Println(file.Filename)
	//set app to processorApp
	service := processorApp{app: app}
	//convert to io reader
	fileReader, err := file.Open()
	if err != nil {
		log.Println(err)
		return "", err
	}

	f, err := excelize.OpenReader(fileReader)
	if err != nil {
		log.Println(err)
		return "", err
	}

	// Get all the rows in the Sheet1.
	rows, err := f.GetRows("Sheet1")
	if err != nil {
		log.Println(err)
		return "", err
	}
	for idx, row := range rows {
		if idx != 0 {
			transData := new(TransData)
			log.Println("Starting row number: ", idx)
			if isStringEmpty(row[TransactionDate]) {
				log.Println("TransactionDate is empty")
			}
			transData.TransactionDate = row[TransactionDate]
			log.Println("TransactionDate", row[TransactionDate])

			if isStringEmpty(row[Amount]) {
				log.Println("Amount is empty")
			} else {
				transData.Amount, err = strconv.ParseFloat(row[Amount], 64)
				if err != nil {
					log.Println(err)
					return "", err
				}
			}
			log.Println("Amount", row[Amount])

			if isStringEmpty(row[PaymentType]) {
				log.Println("PaymentType is empty")
			}
			transData.PaymentType = row[PaymentType]

			log.Println("PaymentType", row[PaymentType])

			if isStringEmpty(row[InvestorName]) {
				log.Println("InvestorName is empty")
			}
			transData.InvestorName = row[InvestorName]

			if isStringEmpty(row[CustomerName]) {
				log.Println("CustomerName is empty")
			}
			transData.CustomerName = row[CustomerName]

			if isStringEmpty(row[StartDate]) {
				log.Println("StartDate is empty")
			}

			transData.StartDate = row[StartDate]

			if isStringEmpty(row[TransactionType]) {
				log.Println("TransactionType is empty")
			}
			transData.TransType = row[TransactionType]

			_, err := service.runDataLoadProcess(*transData)
			if err != nil {
				log.Println(err)
			}

		}
		log.Println("next row")
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
	log.Println("runDataLoadProcess")
	var isNewCustomer = false
	if transData.TransType == LOAN || transData.TransType == PAYMENT {
		//validate customer name
		if transData.CustomerName == "" {
			log.Println("Customer name is empty")
			return "Error: Customer name is empty", nil
		}
		customerRecord, err := service.app.Dao().FindFirstRecordByData(customerCollectionNameOrId, "customerName", transData.CustomerName)
		if err != nil {
			log.Println("Error: Customer record not found")
			isNewCustomer = true
			log.Println("Customer is new")
			customerRecord = service.createNewCustomer(customerRecord, transData)
		}

		if !isNewCustomer {
			var filter = fmt.Sprintf("customerId = '%s' && status = 'Ongoing'", customerRecord.GetId())
			//get loans
			loans, err := service.app.Dao().FindRecordsByFilter(loanCollectionNameOrId, filter, "", 100, 0, nil)
			if err != nil {
				log.Println(err)
				return "Error: Failed to get loans", nil
			}

			//check result
			if len(loans) == 0 && transData.TransType == LOAN {
				log.Println("No loans found")
				//increment renewalCount by 1
				customerRecord.Set("renewalCount", customerRecord.GetInt("renewalCount")+1)
				//save customer record
				err = service.app.Dao().SaveRecord(customerRecord)
				if err != nil {
					log.Println(err)
					return "Error: Failed to save customer record", nil
				}
				return service.processNewLoan(customerRecord, transData)
			} else {
				log.Println("Loan found")
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
			log.Println(err)
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
		log.Println("Error: Investor record not found")
		return "", err
	}

	//check if debit or credit
	if transData.TransType == PAYMENT {
		//update loan record
		loanRecord.Set("remainingBalance", loanRecord.GetFloat("remainingBalance")-transData.Amount)
		loanRecord.Set("paidAmount", loanRecord.GetFloat("paidAmount")+transData.Amount)
		if isLastPayment {
			log.Println("Last payment")
			loanRecord.Set("status", "Completed")
			var targetDate, _ = service.parseDate(transData.TransactionDate, false)

			loanRecord.Set("endDate", targetDate)
		}

		//save loan record
		err = service.app.Dao().SaveRecord(loanRecord)
		if err != nil {
			log.Println(err)
			return "Error saving loan", err
		}
		transData.CashBalance = investorRecord.GetFloat("investmentBalance") + transData.Amount
		//update investor record
		investorRecord.Set("investmentBalance", transData.CashBalance)
		investorRecord.Set("loanedAmount", investorRecord.GetFloat("loanedAmount")-transData.Amount)
		//save investor record
		err = service.app.Dao().SaveRecord(investorRecord)
		if err != nil {
			log.Println(err)
			return "", err
		}

		_, err = service.updateLoanTransaction(transData, loanRecord.GetId(), isLastPayment)
		if err != nil {
			log.Println(err)
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
		log.Println(err)
		return "", err
	}
	//get investor record
	investorRecord, err := service.app.Dao().FindFirstRecordByData(investorCollectionNameOrId, "investorName", transData.InvestorName)
	if err != nil {
		log.Println("Investor record not found")
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
	log.Println("Start date: ", startDate)
	loanRecord.Set("startDate", startDate)
	loanRecord.Set("renewalCount", 0)
	loanRecord.Set("remainingBalance", transData.Amount*interestRate)
	loanRecord.Set("paidAmount", 0)

	//save
	if err := service.app.Dao().SaveRecord(loanRecord); err != nil {
		log.Println(err)
		return "", err
	}

	investorRecord.Set("loanedAmount", investorRecord.GetFloat("loanedAmount")+transData.Amount)
	investorRecord.Set("investmentBalance", investorRecord.GetFloat("investmentBalance")-transData.Amount)
	if err := service.app.Dao().SaveRecord(investorRecord); err != nil {
		log.Println(err)
		return "", err
	}

	return "Success", nil
}

// parse date string to date
func (service *processorApp) parseDate(dateString string, isTransactionDate bool) (types.DateTime, error) {
	date, error := time.Parse("01/02/2006", dateString)
	if error != nil {
		log.Println(error)
		return types.DateTime{}, error
	}
	currentDate := time.Now()
	hour, min, sec := currentDate.Clock()
	millis := currentDate.Nanosecond() / 1000000
	date = time.Date(date.Year(), date.Month(), date.Day(), hour, min, sec, millis, time.Local)
	return types.ParseDateTime(date)
}

func (service *processorApp) createNewCustomer(customerRecord *models.Record, transData TransData) *models.Record {
	//get collection
	customerCollection, err := service.app.Dao().FindCollectionByNameOrId(customerCollectionNameOrId)
	if err != nil {
		log.Println(err)
		return nil
	}
	//create customer record
	customerRecord = models.NewRecord(customerCollection)
	customerRecord.Set("customerName", transData.CustomerName)
	customerRecord.Set("status", "ACTIVE")
	customerRecord.Set("renewalCount", 0)
	//save
	if err := service.app.Dao().SaveRecord(customerRecord); err != nil {
		log.Println("Error saving customer record")
	}
	return customerRecord
}

func (service *processorApp) processInvestorTransaction(transData TransData) (string, error) {
	//get investor record
	investorRecord, err := service.app.Dao().FindFirstRecordByData(investorCollectionNameOrId, "investorName", transData.InvestorName)
	if err != nil {
		log.Println("Investor record not found")
		investorRecord = service.createNewInvestor(transData)
	}
	//check if debit or credit
	if transData.TransType == WITHDRAW || transData.TransType == LOAN {
		transData.CashBalance = investorRecord.GetFloat("investmentBalance") - transData.Amount
		investorRecord.Set("investmentBalance", transData.CashBalance)
		if transData.TransType == WITHDRAW {
			investorRecord.Set("investmentPoolAmount", investorRecord.GetFloat("investmentPoolAmount")-transData.Amount)
		}

	} else if transData.TransType == DEPOSIT || transData.TransType == PAYMENT {
		transData.CashBalance = investorRecord.GetFloat("investmentBalance") + transData.Amount
		investorRecord.Set("investmentBalance", transData.CashBalance)
		if transData.TransType == DEPOSIT {
			investorRecord.Set("investmentPoolAmount", transData.CashBalance)
			transData.Description = investorRecord.GetString("investorName") + " deposited " + strconv.FormatFloat(transData.Amount, 'f', 2, 64)
		}
	}
	//save investor record
	err = service.app.Dao().SaveRecord(investorRecord)
	if err != nil {
		log.Println(err)
		return "", err
	}

	//record to transaction
	_, err = service.createNewTransaction(transData, investorRecord.GetId())
	if err != nil {
		log.Println(err)
		return "", err
	}
	return "Success", nil

}

func (service *processorApp) createNewInvestor(transData TransData) *models.Record {
	//get collection
	investorCollection, err := service.app.Dao().FindCollectionByNameOrId(investorCollectionNameOrId)
	if err != nil {
		log.Println(err)
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
		log.Println(err)
	}
	return investorRecord

}

func (service *processorApp) updateLoanTransaction(transData TransData, loanId string, isLastPayment bool) (*models.Record, error) {
	//filter by customer id
	var filter = fmt.Sprintf("loan = '%s' && type = '%s'", loanId, PENDING)
	log.Println("Filter: ", filter)
	//find transaction record with earliest target date
	transactionRecords, err := service.app.Dao().FindRecordsByFilter(transactionsCollectionNameOrId, filter, "+targetDate", 20, 0)
	if err != nil {
		log.Fatalf("Error finding transaction record: %s", err.Error())
		return nil, err
	}

	transactionCollection, err := service.app.Dao().FindCollectionByNameOrId(transactionsCollectionNameOrId)
	if err != nil {
		log.Fatalf("Error finding transaction collection: %s", err.Error())
		return nil, err
	}

	if len(transactionRecords) == 0 {
		existingRecords, err := service.app.Dao().FindRecordsByFilter(transactionsCollectionNameOrId, fmt.Sprintf("loan = '%s'", loanId), "+transactionDate", 20, 0)
		if err != nil {
			log.Fatalf("Error finding transaction record: %s", err.Error())
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

		saveErr := service.app.Dao().SaveRecord(newTransactionRecord)
		if saveErr != nil {
			log.Fatal(saveErr)
		}

		return newTransactionRecord, nil

	}
	transData.IsAdvancePayment = len(transactionRecords) > 0 && isLastPayment

	if transData.IsAdvancePayment {
		var cashBalance = transData.CashBalance - transData.Amount
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
				log.Println(err)
				return nil, err
			}
		}

		if transData.CashBalance > cashBalance {
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
				log.Fatal(saveErr)
			}
		}
		return transactionRecords[0], nil
	} else {

		var transactionRecord = transactionRecords[0]
		var targetDate, _ = service.parseDate(transData.TransactionDate, true)
		transactionRecord.Set("transactionDate", targetDate)
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
			log.Println(err)
			return nil, err
		}
		return transactionRecord, nil
	}
}

func (service *processorApp) createNewTransaction(transData TransData, investorId string) (*models.Record, error) {
	//create transaction record
	transactionCollection, err := service.app.Dao().FindCollectionByNameOrId(transactionsCollectionNameOrId)
	if err != nil {
		log.Fatalf("Error finding transaction collection: %s", err.Error())
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
		log.Println(err)
		return nil, err
	}
	return transactionRecord, nil

}

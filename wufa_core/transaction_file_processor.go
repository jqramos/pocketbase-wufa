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
	TransactionDate  int = 0
	Amount               = 1
	LoanedAmount         = 2
	PaymentType          = 3
	InvestorName         = 4
	CustomerName         = 5
	IsAdvancePayment     = 6
	StartDate            = 7
	TransactionType      = 8
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
	LoanedAmount     string
	PaymentType      string
	InvestorName     string
	CustomerName     string
	IsAdvancePayment string
	StartDate        string
	TransType        string
}

const customerCollectionNameOrId = "customers"

type processorApp struct {
	app core.App
}

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

	defer func() {
		// Close the spreadsheet.
		if err := f.Close(); err != nil {
			log.Println(err)
		}
	}()

	// Get all the rows in the Sheet1.
	rows, err := f.GetRows("Sheet1")
	if err != nil {
		log.Println(err)
		return "", err
	}

	for idx, row := range rows {
		if idx != 0 {
			transData := new(TransData)

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

			if isStringEmpty(row[LoanedAmount]) {
				log.Println("LoanedAmount is empty")
				transData.LoanedAmount = "0"
			} else {
				transData.LoanedAmount = row[LoanedAmount]
			}

			log.Println("LoanedAmount", row[LoanedAmount])

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

			if isStringEmpty(row[IsAdvancePayment]) {
				log.Println("IsAdvancePayment is empty")
			}

			transData.IsAdvancePayment = row[IsAdvancePayment]

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
				return "", err
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
			//get loans
			loans, err := service.app.Dao().FindRecordsByFilter(loanCollectionNameOrId,
				"status = 'ONGOING'", "", 100)
			if err != nil {
				log.Println(err)
				return "Error: Failed to get loans", nil
			}

			//check result
			if len(loans) == 0 && transData.TransType == LOAN {
				log.Println("No loans found")
				//process new loan
				return service.processNewLoan(customerRecord, transData)
			} else {
				log.Println("Loan found")
				isLastPayment := false
				var balance = loans[0].GetFloat("remainingBalance") - transData.Amount
				if balance == 0 {
					isLastPayment = true
				}
				return service.processExistingLoan(customerRecord, transData, isLastPayment)
			}
		} else {
			return service.processNewLoan(customerRecord, transData)
		}
	} else if transData.TransType == DEPOSIT {
		result, err := service.processInvestorTransaction(transData)
		if err != nil {
			log.Println(err)
			return "Error: Failed to process investor transaction", nil
		}
		return result, nil
	}

	return "", nil
}

func (service *processorApp) processExistingLoan(customerRecord *models.Record, transData TransData, isLastPayment bool) (string, error) {
	//get investor record
	investorRecord, err := service.app.Dao().FindFirstRecordByData(investorCollectionNameOrId, "investorName", transData.InvestorName)
	if err != nil {
		log.Println("Error: Investor record not found")
		return "", err
	}
	//get loan record
	loanRecord, err := service.app.Dao().FindFirstRecordByData(loanCollectionNameOrId, "customerId", customerRecord.GetId())
	if err != nil {
		log.Println("Error: Loan record not found")
		return "", err
	}

	//check if debit or credit
	if transData.PaymentType == "DEBIT" {
		//should not be here
		log.Println("Should not be here")
	} else if transData.PaymentType == CREDIT && transData.TransType == PAYMENT {
		//update loan record
		loanRecord.Set("remainingBalance", loanRecord.GetFloat("remainingBalance")-transData.Amount)
		loanRecord.Set("paidAmount", loanRecord.GetFloat("paidAmount")+transData.Amount)
		if isLastPayment {
			loanRecord.Set("status", "Completed")
			var targetDate, _ = service.parseDate(transData.TransactionDate)

			loanRecord.Set("endDate", targetDate)
		}

		//save loan record
		err = service.app.Dao().SaveRecord(loanRecord)
		if err != nil {
			log.Println(err)
			return "Error saving loan", err
		}

		//update investor record
		investorRecord.Set("investmentBalance", investorRecord.GetFloat("investmentBalance")+transData.Amount)
		investorRecord.Set("loanedAmount", investorRecord.GetFloat("loanedAmount")-transData.Amount)
		//save investor record
		err = service.app.Dao().SaveRecord(investorRecord)
		if err != nil {
			log.Println(err)
			return "", err
		}

		service.updateLoanTransaction(transData)
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
		investorRecord = service.createNewInvestor(transData)
	}

	//create loan record
	loanRecord := models.NewRecord(loanCollection)
	loanRecord.Set("customerId", customerRecord.GetId())
	loanRecord.Set("loanAmount", transData.LoanedAmount)
	loanRecord.Set("amount", transData.LoanedAmount)
	loanRecord.Set("status", "ONGOING")
	loanRecord.Set("investor", investorRecord.GetId())
	var startDate, _ = service.parseDate(transData.StartDate)
	log.Println("Start date: ", startDate)
	loanRecord.Set("startDate", startDate)
	loanRecord.Set("renewalCount", 0)
	loanRecord.Set("remainingBalance", transData.LoanedAmount)
	loanRecord.Set("paidAmount", 0)

	//save
	if err := service.app.Dao().SaveRecord(loanRecord); err != nil {
		log.Println(err)
		return "", err
	}

	return "Success", nil
}

// parse date string to date
func (service *processorApp) parseDate(dateString string) (types.DateTime, error) {
	date, error := time.Parse("01/02/2006", dateString)
	if error != nil {
		log.Println(error)
		return types.DateTime{}, error
	}
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
	//record to transaction
	service.createNewTransaction(transData, investorRecord.GetId())

	//check if debit or credit
	if transData.PaymentType == DEBIT {
		investorRecord.Set("investmentBalance", investorRecord.GetFloat("investmentBalance")-transData.Amount)

	} else if transData.PaymentType == CREDIT {
		investorRecord.Set("investmentBalance", investorRecord.GetFloat("investmentBalance")+transData.Amount)
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
	if transData.PaymentType == CREDIT {
		investorRecord.Set("investmentBalance", transData.Amount)
	}
	investorRecord.Set("loanedAmount", 0)
	//save
	if err := service.app.Dao().SaveRecord(investorRecord); err != nil {
		log.Println(err)
	}
	return investorRecord

}

func (service *processorApp) updateLoanTransaction(transData TransData) *models.Record {
	//filter by customer id
	var filter = fmt.Sprintf("customerName = '%s' && type = '%s'", transData.CustomerName, PENDING)
	log.Println("Filter: ", filter)
	//find transaction record with earliest target date
	transactionRecords, err := service.app.Dao().FindRecordsByFilter(transactionsCollectionNameOrId, filter, "+targetDate", 1)
	if err != nil {
		log.Fatalf("Error finding transaction record: %s", err.Error())
		return nil
	}
	if len(transactionRecords) == 0 {
		log.Println("Error saving transaction record")
		return nil
	}

	var transactionRecord = transactionRecords[0]
	var targetDate, _ = service.parseDate(transData.TransactionDate)
	transactionRecord.Set("transactionDate", targetDate)
	transactionRecord.Set("amount", transData.Amount)
	transactionRecord.Set("loanedAmount", transData.LoanedAmount)
	transactionRecord.Set("type", transData.PaymentType)
	transactionRecord.Set("investorName", transData.InvestorName)
	transactionRecord.Set("customerName", transData.CustomerName)
	//save
	if err := service.app.Dao().SaveRecord(transactionRecord); err != nil {
		log.Println(err)
	}
	return transactionRecord
}

func (service *processorApp) createNewTransaction(transData TransData, investorId string) *models.Record {
	//create transaction record
	transactionCollection, err := service.app.Dao().FindCollectionByNameOrId(transactionsCollectionNameOrId)
	if err != nil {
		log.Fatalf("Error finding transaction collection: %s", err.Error())
		return nil
	}

	transactionRecord := models.NewRecord(transactionCollection)
	var targetDate, _ = service.parseDate(transData.TransactionDate)
	transactionRecord.Set("transactionDate", targetDate)
	transactionRecord.Set("amount", transData.Amount)
	transactionRecord.Set("loanedAmount", transData.LoanedAmount)
	transactionRecord.Set("type", transData.PaymentType)
	transactionRecord.Set("investor", investorId)
	transactionRecord.Set("customerName", transData.CustomerName)
	//save
	if err := service.app.Dao().SaveRecord(transactionRecord); err != nil {
		log.Println(err)
	}
	return transactionRecord

}

package loan_service

import (
	"log"
	"mime/multipart"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/models"
	"github.com/xuri/excelize/v2"
)

//enum for cell column
const (
	TransactionDate int = 0
	Amount = 1
	LoanedAmount = 2
	PaymentType = 3
	InvestorName = 4
	CustomerName = 5
	IsAdvancePayment = 6
	StartDate = 7
)

const (
	CREDIT string = "CREDIT"
	DEBIT = "DEBIT"
	PENDING = "PENDING"
)

//struct for excel data
type TransData struct {
	TransactionDate string
	Amount string
	LoanedAmount string
	PaymentType string
	InvestorName string
	CustomerName string
	IsAdvancePayment string
	StartDate string
}
const customerCollectionNameOrId = "customers"

type processorApp struct {
	app core.App
}


//load excel file to data accepts file
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
		if(idx != 0){
			for i, colCell := range row {		
				//declare empty object transData
				transData := new(TransData)
				//switch case for column
				switch i {
					case TransactionDate:
						transData.TransactionDate = colCell
						log.Println("TransactionDate", colCell)
					case Amount:
						transData.Amount = colCell
						log.Println("Amount", colCell)
					case LoanedAmount:
						transData.LoanedAmount = colCell
						log.Println("LoanedAmount", colCell)
					case PaymentType:
						transData.PaymentType = colCell
						log.Println("PaymentType", colCell)
					case InvestorName:
						transData.InvestorName = colCell
						log.Println("InvestorName", colCell)
					case CustomerName:
						transData.CustomerName = colCell
						log.Println("CustomerName", colCell)
					case IsAdvancePayment:
						transData.IsAdvancePayment = colCell
						log.Println("IsAdvancePayment", colCell)
					case StartDate:
						transData.StartDate = colCell
						log.Println("StartDate", colCell)
				}
				service.runDataLoadProcess(*transData)
			}
		}
        log.Println("next row")
    }
	return "", nil
}

func (service *processorApp) runDataLoadProcess(transData TransData) (string) {
	//validate customer name
	if(transData.CustomerName == ""){
		log.Println("Customer name is empty")
		return "Error: Customer name is empty"
	}
	//find customer record
	customerRecord, err := service.app.Dao().FindFirstRecordByData(customerCollectionNameOrId, "customerName", transData.CustomerName )
	if err != nil {
		log.Println(err)
		return "Error: Customer record not found"
	}
	var isNewCustomer = false
	var isNewLoan = false
	//check if customer record is nil
	if(customerRecord == nil){
		log.Println("Customer is new")
		isNewCustomer = true
		customerRecord = service.createNewCustomer(customerRecord,transData)
		
	} else {
	    result, err := service.processInvestorTransaction(transData)
		if err != nil {
			log.Println(err)
			return "Error: Failed to process investor transaction"
		}
		return result
	}

	if(!isNewCustomer) {
		//get loans
		loans, err := service.app.Dao().FindRecordsByFilter(loanCollectionNameOrId, 
		"status = 'ONGOING'", "", 100)
		if err != nil {
			log.Println(err)
			return "Error: Failed to get loans"
		}

		//check result
		if len(loans) == 0 {
			log.Println("No loans found")
			isNewLoan = true
			return service.processNewLoan(customerRecord, transData)
		} else if(loans >= 1) {
			log.Println("Loan found")
			var isLastPayment = len(loans) == 1
			return service.processExistingLoan(customerRecord, transData, isLastPayment)
		}
	}
}


func (service *processorApp) processExistingLoan(customerRecord *models.Record, transData TransData,isLastPayment bool) (string, error) {
	//get investor record
	investorRecord, err := service.app.Dao().FindFirstRecordByData(investorCollectionNameOrId, "investorName", transData.InvestorName )
	if err != nil {
		log.Println(err)
		return "", err
	}
	//get loan collection
	loanCollection, err := service.app.Dao().FindCollectionByNameOrId(loanCollectionNameOrId)
	if err != nil {
		log.Println(err)
		return "", err
	}
	//get loan record
	loanRecord, err := service.app.Dao().FindFirstRecordByData(loanCollectionNameOrId, "customer", customerRecord.Id() )
	if err != nil {
		log.Println(err)
		return "", err
	}

	//check if debit or credit
	if(transData.PaymentType == "DEBIT") {
		//should not be here
		log.Println("Should not be here")
	} else if(transData.PaymentType == "CREDIT") {
		//update loan record
		loanRecord.Set("remainingBalance", loanRecord.Get("remainingBalance") - transData.Amount)
		loanRecord.Set("paidAmount", loanRecord.Get("paidAmount") + transData.Amount)
		if(isLastPayment) {
			loanRecord.Set("status", "Completed")
			loanRecord.Set("endDate", transData.TransactionDate)
		}

		//save loan record
		err = service.app.Dao().SaveRecord(loanRecord)
		if err != nil {
			log.Println(err)
			return "Error saving loan", err
		}

		//update investor record
		investorRecord.Set("investmentBalance", investorRecord.Get("investmentBalance") + transData.Amount)
		investorRecord.Set("loanedAmount", investorRecord.Get("loanedAmount") - transData.Amount)
		//save investor record
		err = service.app.Dao().SaveRecord(investorRecord)
		if err != nil {
			log.Println(err)
			return "", err
		}

		service.createNewTransaction(transData)
		return "Success: Transaction processed successfully", nil
	}

}

func (service *processorApp) processNewLoan(customerRecord *models.Record, transData TransData) (string, error) {
	//get loan collection
	loanCollection, err := service.app.Dao().FindCollectionByNameOrId(loanCollectionNameOrId)
	if err != nil {
		log.Println(err)
		return "", err
	}
	//create loan record
	loanRecord := models.NewRecord(loanCollection)
	loanRecord.Set("customer", customerRecord.Id())
	loanRecord.Set("loanAmount", transData.LoanedAmount)
	loanRecord.Set("status", "ONGOING")
	loanRecord.Set("investor", transData.InvestorName)
	//convert string to time
	var startDate = types.ParseDateTime(transData.StartDate)
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

func (service *processorApp) createNewCustomer(customerRecord *models.Record,transData TransData) (*models.Record){
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
		log.Println(err)
	}
	return customerRecord
}

func (service *processorApp) processInvestorTransaction(transData TransData) (string, error) {
	//get investor record
	investorRecord, err := service.app.Dao().FindFirstRecordByData(investorCollectionNameOrId, "investorName", transData.InvestorName )
	if err != nil {
		log.Println(err)
		return "Error: Investor record not found", err
	}
	//check if investor record is nil
	if(investorRecord == nil){
		log.Println("Investor record not found")
		investorRecord = service.createNewInvestor(transData)
	}
	//record to transaction
	transactionRecord := service.createNewTransaction(transData)
	
	//check if debit or credit
	if(transData.PaymentType == DEBIT){
		investorRecord.Set("investmentBalance", investorRecord.getFloat("investmentBalance") - transData.Amount)

	} else if(transData.PaymentType == CREDIT){
		investorRecord.Set("investmentBalance", investorRecord.getFloat("investmentBalance") + transData.Amount)
	}

	return "Success", nil

    
}


func (service *processorApp) createNewInvestor(transData TransData) (*models.Record) {
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
	if(transData.PaymentType == CREDIT){
		investorRecord.Set("investmentBalance", transData.Amount)
	}
	investorRecord.Set("loanedAmount", 0)
	//save
	if err := service.app.Dao().SaveRecord(investorRecord); err != nil {
		log.Println(err)
	}
	return investorRecord

}

func (service *processorApp) createNewTransaction(transData TransData) (*models.Record) {
	//save transaction
	transactionCollection, err := service.app.Dao().FindCollectionByNameOrId(transactionCollectionNameOrId)
	if err != nil {
		log.Println(err)
		return nil
	}
	//create transaction record
	transactionRecord := models.NewRecord(transactionCollection)
	transactionRecord.Set("transactionDate", transData.TransactionDate)
	transactionRecord.Set("amount", transData.Amount)
	transactionRecord.Set("loanedAmount", transData.LoanedAmount)
	transactionRecord.Set("paymentType", transData.PaymentType)
	transactionRecord.Set("investorName", transData.InvestorName)
	transactionRecord.Set("customerName", transData.CustomerName)
	//save
	if err := service.app.Dao().SaveRecord(transactionRecord); err != nil {
		log.Println(err)
	}
	return transactionRecord
}
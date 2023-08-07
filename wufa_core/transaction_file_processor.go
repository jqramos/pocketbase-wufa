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
)

const (
	Credit string = "CREDIT"
	Debit = "DEBIT"
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
	//check if customer record is nil
	if(customerRecord == nil){
		log.Println("Customer record not found")
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
			return "Error: No loans found"
		}
	}
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
		investorRecord = service.createNewInvestor(transData.InvestorName)
	}
}


func (service *processorApp) createNewInvestor(investorName string) (*models.Record) {
	//get collection
	investorCollection, err := service.app.Dao().FindCollectionByNameOrId(investorCollectionNameOrId)
	if err != nil {
		log.Println(err)
		return nil
	}
	//create investor record
	investorRecord := models.NewRecord(investorCollection)
	investorRecord.Set("investorName", investorName)
	investorRecord.Set("status", "ACTIVE")
	investorRecord.Set("investmentBalance", 0)
	investorRecord.Set("loanedAmount", 0)
	//save
	if err := service.app.Dao().SaveRecord(investorRecord); err != nil {
		log.Println(err)
	}
	return investorRecord

}
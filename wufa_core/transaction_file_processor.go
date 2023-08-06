package loan_service
import (
	"log"
)

//LoadCSVFileToData function accepts records from csv file and processes them
func LoadCSVFileToData(records [][]string)  (string, error) {
	//loop through records
	for _, record := range records {
		//log record
		log.Println("Record: ", record)
	}




	//return success
	return "failed", nil
}
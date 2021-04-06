package contracts
import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"
	"log"

	"math"
	"math/rand"
	"github.com/hyperledger/fabric-chaincode-go/shim"

	"github.com/hyperledger/fabric-chaincode-go/pkg/cid"
	"github.com/hyperledger/fabric-contract-api-go/contractapi"
)

type NewContract struct {
	contractapi.Contract
}

type UTXO struct {
	Key    string `json:"utxo_key"`
	Owner  string `json:"owner"`
	Amount int    `json:"amount"`
}


var (
	ErrOldID                 = errors.New("This PPA's ID already exists")
	ErrAtraso                = errors.New("This PPA will be considered in default")
	ErrNumMax                = errors.New("Not on correct period or achieved max number of contracts")
	ErrWrongPeriod           = errors.New("You are searching in a wrong period")
	ErrNotAValidFormatClient = errors.New("Client name hasnt a valid format")
	ErrNoFarmer              = errors.New("The identity should be a farmer to execute the transaction")
	ErrNoOriginator              = errors.New("The identity should be an originator to execute the transaction")
	ErrNoSpv=errors.New("The identity should be a SPV to execute the transaction")
	ErrNoPeriod=errors.New("You are not allowed to write in this period")
	ErrFarmerPeriod=errors.New("This client has already submit a payment for this period")
)

func Transfer(ctx contractapi.TransactionContextInterface, utxoInputKeys []string, amount int) (*UTXO, error) {
	hasOU, err := cid.HasOUValue(ctx.GetStub(), "client1")
	if err != nil {
		return nil, err
	}
	if !hasOU {
		return nil, ErrNoFarmer
	}
	clientMSPID, err := ctx.GetClientIdentity().GetMSPID()
	if err != nil {
		return nil, fmt.Errorf("failed to get MSPID: %v", err)
	}
	if clientMSPID != "farmerMSP" {
		return nil, fmt.Errorf("client is not authorized to receive new tokens")
	}


	// Get ID of submitting client identity
	clientID, err := ctx.GetClientIdentity().GetID()
	if err != nil {
		return nil, fmt.Errorf("failed to get client id: %v", err)
	}

	// Validate and summarize utxo inputs
	utxoInputs := make(map[string]*UTXO)
	var totalInputAmount int
	for _, utxoInputKey := range utxoInputKeys {
		if utxoInputs[utxoInputKey] != nil {
			return nil, fmt.Errorf("the same utxo input can not be spend twice")
		}

		utxoInputCompositeKey, err := ctx.GetStub().CreateCompositeKey("utxo", []string{clientID, utxoInputKey})
		if err != nil {
			return nil, fmt.Errorf("failed to create composite key: %v", err)
		}

		// validate that client has a utxo matching the input key
		valueBytes, err := ctx.GetStub().GetState(utxoInputCompositeKey)
		if err != nil {
			return nil, fmt.Errorf("failed to read utxoInputCompositeKey %s from world state: %v", utxoInputCompositeKey, err)
		}

		if valueBytes == nil {
			return nil, fmt.Errorf("utxoInput %s not found for client %s", utxoInputKey, clientID)
		}

		//amount, _ := strconv.Atoi(string(valueBytes)) // Error handling not needed since Itoa() was used when setting the utxo amount, guaranteeing it was an integer.

		utxoInput := &UTXO{
			Key:    utxoInputKey,
			Owner:  clientID,
			Amount: amount,
		}

		totalInputAmount += amount
		utxoInputs[utxoInputKey] = utxoInput
	}

	for _, utxoInput := range utxoInputs {

		utxoInputCompositeKey, err := ctx.GetStub().CreateCompositeKey("utxo", []string{utxoInput.Owner, utxoInput.Key})
		if err != nil {
			return nil, fmt.Errorf("failed to create composite key: %v", err)
		}

		err = ctx.GetStub().DelState(utxoInputCompositeKey)
		if err != nil {
			return nil, err
	}
	log.Printf("utxoInput deleted: %+v", utxoInput)
	}

	utxoOutput:=new(UTXO)
	utxoOutput.Key = ctx.GetStub().GetTxID() + ".0"
	mspid:="originatorMSP"
	utxoOutput.Owner=mspid
	utxoOutput.Amount=totalInputAmount
	utxoOutputCompositeKey, err := ctx.GetStub().CreateCompositeKey("utxo", []string{utxoOutput.Owner, utxoOutput.Key})
	if err != nil {
		return nil, fmt.Errorf("failed to create composite key: %v", err)
	}

	err = ctx.GetStub().PutState(utxoOutputCompositeKey, []byte(strconv.Itoa(utxoOutput.Amount)))
	if err != nil {
		return nil, err
	}
	log.Printf("utxoOutput created: %+v", utxoOutput)

	return utxoOutput, nil
}

//ejemplo fabric-samples
// ClientUTXOs returns all UTXOs owned by the calling client
//cambiar a interfaz
func ClientUTXOs(ctx contractapi.TransactionContextInterface) ([]string, error) {

	// Get ID of submitting client identity
	clientID, err := ctx.GetClientIdentity().GetID()
	if err != nil {
		return nil, fmt.Errorf("failed to get client id: %v", err)
	}

	// since utxos have a composite key of owner:utxoKey, we can query for all utxos matching owner:*
	utxoResultsIterator, err := ctx.GetStub().GetStateByPartialCompositeKey("utxo", []string{clientID})
	if err != nil {
		return nil, err
	}
	defer utxoResultsIterator.Close()

	var newUTXO UTXO
	var utxos []*UTXO
	for utxoResultsIterator.HasNext() {
		utxoRecord, err := utxoResultsIterator.Next()
		if err != nil {
			return nil, err
		}

		// composite key is expected to be owner:utxoKey
		_, compositeKeyParts, err := ctx.GetStub().SplitCompositeKey(utxoRecord.Key)
		if err != nil {
			return nil, err
		}

		if len(compositeKeyParts) != 2 {
			return nil, fmt.Errorf("expected composite key with two parts (owner:utxoKey)")
		}

		utxoKey := compositeKeyParts[1] // owner is at [0], utxoKey is at[1]

		if utxoRecord.Value == nil {
			return nil, fmt.Errorf("utxo %s has no value", utxoKey)
		}

		amount, _ := strconv.Atoi(string(utxoRecord.Value)) // Error handling not needed since Itoa() was used when setting the utxo amount, guaranteeing it was an integer.

		utxo := &UTXO{
			Key:    utxoKey,
			Owner:  clientID,
			Amount: amount,
		}
		newUTXO.Key=utxoKey
		newUTXO.Amount=amount
		utxos = append(utxos, utxo)
	}
	return []string{newUTXO.Key,strconv.Itoa(newUTXO.Amount)}, nil
}


func AfterTransaction(ctx contractapi.TransactionContextInterface) error{
	idUTXO,err:=s.ClientUTXOs(ctx)
	if err!=nil{
		return fmt.Errorf("Error: %v",err)
	}
	value:=idUTXO[0]
	log.Printf("valor de la clave: %v",value)
	cant,_:=strconv.Atoi(idUTXO[1])
	_,err=s.Transfer(ctx,[]string{value},cant)
	return err
}

package main

// Especially useful if you don't use Go as a main to create transactions
/*
   POST /send-tx
   {
   	"tx": "<base64_encoded_tx>"
   }

   200 = success
   400 = malformed request
   500 = transaction failed to be sent
*/

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"log"
	"net/http"

	"github.com/gagliardetto/solana-go/rpc"
	"github.com/gorilla/mux"
	"github.com/qg5/go-solana-tpu/tpu"
)

var (
	port        string
	rpcEndpoint string
)

func main() {
	flag.StringVar(&port, "port", "3333", "The port that the server will be runnning under")
	flag.StringVar(&rpcEndpoint, "rpc", rpc.DevNet_RPC, "Rpc URL to connect to")
	flag.Parse()

	conn := rpc.New(rpcEndpoint)

	tpuClient, err := tpu.New(conn, nil)
	if err != nil {
		log.Fatal(err)
	}

	r := mux.NewRouter()
	r.HandleFunc("/send-tx", handleSendTx(tpuClient)).Methods("POST")

	log.Fatal(http.ListenAndServe(":"+port, r))
}

type TransactionRequest struct {
	Tx string `json:"tx"`
}

func handleSendTx(tpuClient *tpu.TPUClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		var payload TransactionRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "invalid json provided", http.StatusBadRequest)
			return
		}

		serializedTx, err := base64.StdEncoding.DecodeString(payload.Tx)
		if err != nil {
			http.Error(w, "failed to decode base64", http.StatusBadRequest)
			return
		}

		tpuClient.Update()

		if err := tpuClient.SendRawTransaction(serializedTx); err != nil {
			http.Error(w, "couldnt send transaction", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	}
}

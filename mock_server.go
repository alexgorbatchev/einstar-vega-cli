package main

import (
	"encoding/binary"
	"fmt"
	"log"
	"net/http"

	"github.com/fxamacker/cbor/v2"
)

func main() {
	http.HandleFunc("/connect", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("connect success"))
	})

	http.HandleFunc("/request/deviceInfo", func(w http.ResponseWriter, r *http.Request) {
		payload := map[string]interface{}{
			"result": map[string]interface{}{
				"firmwareVersion": "1.3.0-22",
				"model":           "Vega",
				"ssdPath":         "/mnt/sdcard",
				"batteryLevel":    85,
			},
		}
		b, _ := cbor.Marshal(payload)
		_, _ = w.Write(b)
	})

	http.HandleFunc("/request/projectsInfo", func(w http.ResponseWriter, r *http.Request) {
		payload := map[string]interface{}{
			"result": map[string]interface{}{
				"Project1": map[string]interface{}{
					"Path":     "/mnt/sdcard/Project1",
					"DateTime": "2023-10-27T10:00:00Z",
				},
				"Project2": map[string]interface{}{
					"Path":     "/mnt/sdcard/Project2",
					"DateTime": "2023-10-28T11:00:00Z",
				},
			},
		}
		b, _ := cbor.Marshal(payload)
		_, _ = w.Write(b)
	})

	http.HandleFunc("/request/listFilePaths", func(w http.ResponseWriter, r *http.Request) {
		// Just parse path, return dummy files
		payload := map[string]interface{}{
			"result": []interface{}{
				0, // some status
				[]interface{}{
					"/mnt/sdcard/Project1/mesh.beb",
					"/mnt/sdcard/Project1/preview.png",
					"/mnt/sdcard/Project1/frame0/DepthImg_0.dat",
				},
			},
		}
		b, _ := cbor.Marshal(payload)
		_, _ = w.Write(b)
	})

	http.HandleFunc("/file/info", func(w http.ResponseWriter, r *http.Request) {
		// Mock size 1024 bytes
		_ = binary.Write(w, binary.BigEndian, uint64(1024))
	})

	http.HandleFunc("/file/content", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(make([]byte, 1024))
	})

	fmt.Println("Mock Vega server listening on :8080...")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
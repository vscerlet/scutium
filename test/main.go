package main

import (
	"github.com/vscerlet/scutium"
	"log"
	"net"
)

func testHandler(conn net.Conn, pkg []byte) error {
	println("Сообщение:", string(pkg))

	return nil
}

func main() {
	server := scutium.NewServer("127.0.0.1:25585", "tcp")

	server.On(0x0, testHandler)

	err := server.Listen()
	if err != nil {
		log.Fatal(err)
	}
}

package main

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strings"
)

const constPasswordSalt = "GY=B[+inIGy,W5@U%kwP/wWrw%4uQ?6|8P$]9{X=-XY:LO6*1cG@P-+`<s=+TL#N"

func main() {

	reader := bufio.NewReader(os.Stdin)

	for {

		text, _ := reader.ReadString('\n')

		text = strings.TrimSpace(text)

		if text == "" || text == "exit" {
			break
		}

		h := sha1.New()
		io.WriteString(h, text+constPasswordSalt)
		passwordHashed := base64.URLEncoding.EncodeToString(h.Sum(nil))

		fmt.Println(passwordHashed)
	}

}

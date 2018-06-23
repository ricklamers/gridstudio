# while true
# do go build main.go cell.go parse.go hub.go client.go python.go &&
# ./main &&
# echo $!
# wait $!
# done
go run main.go tests.go cell.go parse.go hub.go client.go python.go -addr=:4000 

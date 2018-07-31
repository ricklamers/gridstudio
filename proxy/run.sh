go build manager.go

if [[ "$OSTYPE" == "msys" ]]; then
	docker kill $(docker ps -q)
	./manager.exe
else
	sudo docker kill $(sudo docker ps -q)
	sudo ./manager
fi


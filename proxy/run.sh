go build manager.go

if [[ "$OSTYPE" == "msys" ]]; then
	./manager.exe
else
	sudo ./manager
fi


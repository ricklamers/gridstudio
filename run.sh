# go build manager.go

# if [[ "$OSTYPE" == "msys" ]]; then
# 	docker kill $(docker ps -q)
# 	./manager.exe
# else
# 	sudo docker kill $(sudo docker ps -q)
# 	sudo ./manager
# fi

unameOut="$(uname -s)"
case "${unameOut}" in
    Linux*)     machine=Linux;;
    Darwin*)    machine=Mac;;
    CYGWIN*)    machine=Cygwin;;
    MINGW*)     machine=MinGw;;
    *)          machine="UNKNOWN:${unameOut}"
esac

if [[ "${machine}" == "MinGw" ]]; then
    docker run --name=gridstudio --rm=false -v /${PWD}/grid-app:/home/source -v /${PWD}/grid-app/proxy/userdata:/home/userdata -p 8080:8080 -p 4430:4430 ricklamers/gridstudio:release
else
    docker run --name=gridstudio --rm=false -v $PWD/grid-app:/home/source -v $PWD/grid-app/proxy/userdata:/home/userdata -p 8080:8080 -p 4430:4430 ricklamers/gridstudio:release
fi

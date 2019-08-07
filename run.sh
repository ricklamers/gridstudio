#!/usr/bin/env bash

if [[ "$OSTYPE" == "msys" ]]; then

	if [ ! "$(docker ps -a | grep gridstudio)" ]; then

		WIN_PWD=$PWD
		WIN_PWD=$(echo $WIN_PWD | sed -r 's/[/]+/\\/g')
		WIN_PWD=$(echo $WIN_PWD | sed -r 's/\\([a-z])\\+/\U\1:\\/g')

		docker run --name=gridstudio --rm=false -v $WIN_PWD\\grid-app:/home/source -v $WIN_PWD\\grid-app\\proxy\\userdata:/home/userdata -p 8080:8080 -p 4430:4430 ricklamers/gridstudio:release
	else
		echo "gridstudio container detected - starting container - want to do a full restart? Run destroy.sh first."
		docker start gridstudio
	fi
else
	if [ ! "$(docker ps -a | grep gridstudio)" ]; then
		docker run --name=gridstudio --rm=false -v $PWD/grid-app:/home/source -v $PWD/grid-app/proxy/userdata:/home/userdata -p 8080:8080 -p 4430:4430 ricklamers/gridstudio:release
	else
		echo "gridstudio container detected - starting container - want to do a full restart? Run destroy.sh first."
		docker start gridstudio
	fi
fi

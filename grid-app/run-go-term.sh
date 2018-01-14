#!/bin/bash

# Start the first process
# mkdir /home/run/
# echo "Created directory /home/run"
cp -Rf /home/source/terminal-server/* /home/run/terminal-server
# rsync -av /home/source/terminal-server /home/run/terminal-server
echo "Copied directory"
# cat /home/run/terminal-server/app.js
cd /home/run/terminal-server/
echo "Changed directory"
# npm install
# echo "Installed terminal-server"
node app.js &
status=$?
echo "Node terminal-server running"
if [ $status -ne 0 ]; then
  echo "Failed to start Node Term server: $status"
  exit $status
fi

cd /home/source/
# Start the second process
go run main.go cell.go parse.go hub.go client.go python.go &
status=$?
if [ $status -ne 0 ]; then
  echo "Failed to start Go webserver: $status"
  exit $status
fi


# Naive check runs checks once a minute to see if either of the processes exited.
# This illustrates part of the heavy lifting you need to do if you want to run
# more than one service in a container. The container will exit with an error
# if it detects that either of the processes has exited.
# Otherwise it will loop forever, waking up every 60 seconds

while sleep 60; do
  ps aux |grep main |grep -q -v grep
  PROCESS_1_STATUS=$?
  ps aux |grep node |grep -q -v grep
  PROCESS_2_STATUS=$?
  # If the greps above find anything, they will exit with 0 status
  # If they are not both 0, then something is wrong
  if [ $PROCESS_1_STATUS -ne 0 -o $PROCESS_2_STATUS -ne 0 ]; then
    echo "One of the processes has already exited."
    exit -1
  fi
done
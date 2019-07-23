echo "--> Updating terminal app.js"
cp /home/source/terminal-server/app.js /home/run/terminal-server/app.js

echo "--> Run manager proxy, starting manager.go (compiling with go run ...)"
go run manager.go
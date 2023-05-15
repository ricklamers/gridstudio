<img src='https://github.com/ricklamers/gridstudio/assets/1309307/147b8ffd-8843-4a4a-b42c-e28805e9d3e7' width='200px' style='margin-bottom: 30px;'>

Grid studio is a web-based spreadsheet application with full integration of the Python programming language.


https://user-images.githubusercontent.com/1309307/233638107-f568519d-4581-4e20-92a7-b61e628d5fef.mp4


It intends to provide an integrated workflow for loading, cleaning, manipulating, and visualizing data. This is achieved through a spreadsheet backend written in Go with integration of the Python runtime to manipulate its contents.

### Architecture overview
The application is structured in two parts:

1. The (centralized) workspace manager
    1. CRUD interface for creating, copying, editing and deleting workspaces.
    1. Proxy to send traffic to the right workspace environment (part 2)
1. Workspace Go execution environment
    1. Go cell parsing and evaluating spreadsheet backend
    1. Node.js terminal session
    1. Python interpreter integration

For more details about each part check out the code in the repository. If anything is unclear (or unreadable - not all code is equally pretty!) make an issue and details will be provided.

### Features

#### Spreadsheet functions that you know
https://user-images.githubusercontent.com/1309307/233638180-87c4375d-20b6-46da-9049-8ad60ff32beb.mp4

#### Powerful scripting, fully integrated
https://user-images.githubusercontent.com/1309307/233638234-6c282006-c615-41ca-bfff-5f8cb9c2dab5.mp4

#### Run any command on Ubuntu Linux
https://user-images.githubusercontent.com/1309307/233638276-9ff2a532-3940-49ea-b152-ffa8ded3c4d0.mp4

### Installation
To run Grid studio locally refer to the <a href="https://github.com/ricklamers/gridstudio/wiki/Installation">Installation</a> page of the Wiki.

It comes down to pulling the latest Grid studio Docker image that has all dependencies configured (mainly: Go language, Python 3 with packages, Node.js) and starting the Docker container.

For more information check out our <a href="https://github.com/ricklamers/gridstudio/wiki">Wiki</a>.


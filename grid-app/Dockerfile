FROM golang:1.9

WORKDIR /home/source/

COPY . /home/source/

RUN apt update
RUN apt install python3 -y

RUN go-wrapper download   # "go get -d -v ./..."
# RUN go-wrapper install    # "go install -v ./..."

# RUN go get github.com/codegangsta/gin

# CMD ["go-wrapper", "run"] # "go run app"
CMD ["bash","run.sh"]
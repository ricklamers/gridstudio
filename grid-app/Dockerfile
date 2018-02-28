FROM golang:1.9

WORKDIR /home/source/

COPY . /home/source/

RUN apt update
RUN apt install python3 -y
RUN curl -sL https://deb.nodesource.com/setup_8.x | bash -
RUN apt install -y nodejs
RUN apt install -y build-essential
RUN apt install -y python3-pip
RUN pip3 install numpy
RUN pip3 install pandas

RUN mkdir /home/run/
COPY ./terminal-server/ /home/run/terminal-server/

# create work directory
RUN mkdir /home/user/

WORKDIR /home/run/terminal-server/
RUN npm install

WORKDIR /home/source/

RUN go-wrapper download   # "go get -d -v ./..."

EXPOSE 8080
EXPOSE 3000

CMD ["bash","run-go-term.sh"]
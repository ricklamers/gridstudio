FROM ubuntu:16.04
LABEL maintainer "NVIDIA CORPORATION <cudatools@nvidia.com>"

# RUN NVIDIA_GPGKEY_SUM=d1be581509378368edeec8c1eb2958702feedf3bc3d17011adbf24efacce4ab5 && \
#     NVIDIA_GPGKEY_FPR=ae09fe4bbd223a84b2ccfce3f60f4b3d7fa2af80 && \
#     apt-key adv --fetch-keys http://developer.download.nvidia.com/compute/cuda/repos/ubuntu1604/x86_64/7fa2af80.pub && \
#     apt-key adv --export --no-emit-version -a $NVIDIA_GPGKEY_FPR | tail -n +5 > cudasign.pub && \
#     echo "$NVIDIA_GPGKEY_SUM  cudasign.pub" | sha256sum -c --strict - && rm cudasign.pub && \
#     echo "deb http://developer.download.nvidia.com/compute/cuda/repos/ubuntu1604/x86_64 /" > /etc/apt/sources.list.d/cuda.list && \
#     echo "deb http://developer.download.nvidia.com/compute/machine-learning/repos/ubuntu1604/x86_64 /" > /etc/apt/sources.list.d/nvidia-ml.list

# ENV CUDA_VERSION 9.0.176

# ENV CUDA_PKG_VERSION 9-0=$CUDA_VERSION-1
# RUN apt-get update && apt-get install -y --no-install-recommends \
#         cuda-cudart-$CUDA_PKG_VERSION && \
#     ln -s cuda-9.0 /usr/local/cuda && \
#     rm -rf /var/lib/apt/lists/*

# # nvidia-docker 1.0
# LABEL com.nvidia.volumes.needed="nvidia_driver"
# LABEL com.nvidia.cuda.version="${CUDA_VERSION}"

# RUN echo "/usr/local/nvidia/lib" >> /etc/ld.so.conf.d/nvidia.conf && \
#     echo "/usr/local/nvidia/lib64" >> /etc/ld.so.conf.d/nvidia.conf

# ENV PATH /usr/local/nvidia/bin:/usr/local/cuda/bin:${PATH}
# ENV LD_LIBRARY_PATH /usr/local/nvidia/lib:/usr/local/nvidia/lib64

# # nvidia-container-runtime
# ENV NVIDIA_VISIBLE_DEVICES all
# ENV NVIDIA_DRIVER_CAPABILITIES compute,utility
# ENV NVIDIA_REQUIRE_CUDA "cuda>=9.0"

RUN apt-get clean && apt-get update && apt-get -y install software-properties-common && add-apt-repository ppa:deadsnakes/ppa -y && add-apt-repository ppa:gophers/archive -y && apt-get update && apt-get install -y locales && apt-get -y install golang-1.10-go && apt-get install -y python3.7

RUN sed -i -e 's/# en_US.UTF-8 UTF-8/en_US.UTF-8 UTF-8/' /etc/locale.gen && \
    locale-gen
ENV LANG en_US.UTF-8  
ENV LANGUAGE en_US:en  
ENV LC_ALL en_US.UTF-8  

# get rid of apt-get dialog warnings
RUN echo 'debconf debconf/frontend select Noninteractive' | debconf-set-selections

# install golang
ENV PATH /usr/lib/go-1.10/bin:${PATH}

# copy over all files to /home/source/
WORKDIR /home/source/
COPY . /home/source/

# install python + dependencies + nodejs
RUN apt-get install curl -y && apt-get install git -y && curl -sL https://deb.nodesource.com/setup_10.x | bash - && apt-get install -y nodejs && apt-get install -y build-essential && apt-get install -y python3-pip && pip3 install --upgrade pip && apt-get install -y python3-tk && pip3 install numpy && pip3 install pandas && pip3 install matplotlib && pip3 install scipy && pip3 install scikit-learn && pip3 install tensorflow

# create /home/run/ directory to run terminal nodejs project from
RUN mkdir /home/run/
COPY ./terminal-server/ /home/run/terminal-server/

# install required NPM packages for term.js
WORKDIR /home/run/terminal-server/
RUN npm install

# create work directory
RUN mkdir /home/user/

# set working directory to source to install go dependencies
WORKDIR /home/source/

RUN go get -d -v ./...

# expose ports
EXPOSE 8080
EXPOSE 3000

CMD ["bash","run-go-term.sh"]
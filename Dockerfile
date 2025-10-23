FROM ros:humble-ros-core AS builder

RUN apt-get update && apt-get install -y --no-install-recommends --fix-missing \
    build-essential gcc-11 g++-11 clang libclang-dev curl wget git nano ca-certificates \
    python3-colcon-common-extensions \
    ros-humble-rosidl-generator-dds-idl \
    ros-humble-rosbag2  \
    ros-humble-rosbag2-storage-mcap \
    ros-humble-rmw-cyclonedds-cpp \
    ros-humble-cyclonedds \
    ros-humble-rmw-fastrtps-cpp \
    && rm -rf /var/lib/apt/lists/*

RUN update-ca-certificates

ENV GO_VERSION=1.25.3

RUN mkdir /opt/app
WORKDIR /opt/app

RUN wget https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz
RUN rm -rf /usr/local/go && tar -C /usr/local -xzf go${GO_VERSION}.linux-amd64.tar.gz && export PATH=$PATH:/usr/local/go/bin && go version

COPY go.mod go.sum ./
RUN export PATH=$PATH:/usr/local/go/bin && go mod download && go get github.com/okieraised/rclgo/core

RUN mkdir ./internal

COPY custom_interfaces ./custom_interfaces
COPY build_custom_interfaces.sh ./
RUN chmod +x build_custom_interfaces.sh && ./build_custom_interfaces.sh

COPY ./*.go ./
COPY ./internal ./internal

RUN . /opt/ros/humble/setup.sh && \
    export PATH=$PATH:/usr/local/go/bin && \
    go run github.com/okieraised/rclgo/core/cmd/ros2gen generate -d ./internal/ros_msgs

RUN export PATH=$PATH:/usr/local/go/bin && go build -ldflags="-s -w" -o monitoring-agent .

CMD ["bash"]


FROM ros:humble-ros-core AS runtime

RUN apt-get update && apt-get install -y --no-install-recommends --fix-missing \
    build-essential gcc-11 g++-11 clang libclang-dev curl wget git nano ca-certificates ffmpeg tzdata \
    python3-colcon-common-extensions \
    ros-humble-rosidl-generator-dds-idl \
    ros-humble-rosbag2  \
    ros-humble-rosbag2-storage-mcap \
    ros-humble-rmw-cyclonedds-cpp \
    ros-humble-cyclonedds \
    ros-humble-rmw-fastrtps-cpp \
    && rm -rf /var/lib/apt/lists/*

ENV TZ=Asia/Ho_Chi_Minh
RUN ln -snf /usr/share/zoneinfo/$TZ /etc/localtime && echo $TZ > /etc/timezone

RUN mkdir /opt/app
WORKDIR /opt/app

COPY --from=builder /opt/ros/humble /opt/ros/humble
COPY --from=builder /opt/app/monitoring-agent /opt/app/monitoring-agent
COPY ./fastdds_profile.xml /opt/app/fastdds_profile.xml
ENV FASTRTPS_DEFAULT_PROFILES_FILE=/opt/app/fastdds_profile.xml

RUN touch .env
CMD ["./monitoring-agent"]
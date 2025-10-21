protoc --go_out=./internal/server/grpc_server/routes --go_opt=paths=source_relative \
 --proto_path=./proto_definitions/ \
 --go-grpc_out=./internal/server/grpc_server/routes --go-grpc_opt=paths=source_relative \
 ./proto_definitions/ros_interface.proto
# FTTP - A simple HTTP/1.1 and HTTP/2 Server

## 1. Description

This project implements an HTTP/2 server with support for frame multiplexing and handling. 
The server is capable of handling HTTP/2 frame parsing, response handling, and channel-based frame transmission.
It also implements HTTP/1, with support for chunked encoding and pipelining.

## 2. Setup and Installation

1. **Clone the repository**:
    ```shell
      git clone https://github.com/Shu-AFK/fttp
      cd fttp
    ```
2. **Install dependencies**: Ensure you have Go installed. Then run: 
    ```shell
      go mod tidy
    ```
3. **Create TLS Certificates**:
    ```shell
      go run tools/certificateGenerator/certgen.go -org "<Organisation Name>" -cn "<Domain Name>" -on "<Department Name>" -ip "<IP>" -name "<Name for the server>"
    ```
4. **Running the Server**: Make sure to use the previously generated .pem files
    ```shell
      go run main.go -cert <name>-cert.pem -key <name>-key.pem 
   ```

## 3. License

This project is distributed under the MIT License. See our [LICENSE](LICENSE.md) for more details.
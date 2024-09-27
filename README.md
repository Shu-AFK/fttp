# FTTP - A Configurable Reverse Proxy

## 1. Description

This project implements a simple reverse proxy server capable of forwarding incoming 
HTTP requests to different backend servers. The proxy server supports customizable 
routing based on URL paths, and is designed to intelligently handle and forward client 
HTTP/1.1 and HTTP/2 requests.

Based on the path of an incoming request, the proxy server redirects the request to a 
specific backend server, thereby channeling different requests to different backend 
applications. This allows your backends to remain modularized and isolated while 
presenting a unified front via the reverse proxy.

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
3. **Generate TLS Certificates**: (Optional)
    ```shell
      go run tools/certificateGenerator/certgen.go -org "<Organisation Name>" -cn "<Domain Name>" -on "<Department Name>" -ip "<IP>" -name "<Name for the server>"
    ```
4. **Configure the Reverse Proxy**:
   To see all the options, take a look at the configs in the [example configs](example_configs/) or reference [CONFIG.md](CONFIG.md)
5. **Running the Reverse Proxy**:
    ```shell
      go run main.go -cert <name>-cert.pem -key <name>-key.pem -config <name>.yaml
   ```

## 3. License

This project is distributed under the MIT License. See our [LICENSE](LICENSE.md) for more details.
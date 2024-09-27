# Configuring the reverse proxy

Here is a breakdown of what each segment means in the configuration file:

- **server**: This section is where you define server properties:
  - **port**: The port on which the reverse proxy should listen
  - **routes**: An array of routes that the proxy should handle
    - **path**: The incoming path to match.
    - **host**: The domain name or IP address and port of the backend server.
    - **target_path**: The path on the backend server to redirect to.

- **add_header**: Define any additional headers that should be included in all responses from the proxy. The field name should be the header name, and the value should be an array of header values.

- **caching**: Configuration for response caching:
  - **enabled**: Whether response caching is enabled. Defaults to false.
  - **ttl**: The time-to-live (TTL) for the cache, in seconds. Ignored if enabled is false.

- **blacklist**: An array of IPs that are to be blacklisted by the reverse proxy. Blacklisted IPs will not be able to access the proxy.

- **logger**: Configuration options for the logging system.
  - **level**: The level of log messages to capture (e.g., "debug", "info", "warn", "error").
  - **file**: The file to which logs should be written. If not provided, logs will be output to stdout.

To see concrete example configs, make sure to look at the [example configs](example_configs)
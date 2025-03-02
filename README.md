# hrtester - HTTP roundtrip tester

A lightweight HTTP benchmarking tool. It includes three main components:

- **Tester**: Sends predefined HTTP requests and logs results.
- **Collector**: Aggregates test results.
- **Mock Service**: Simulates responses for testing purposes.

## Installation

Clone the repository:
```sh
git clone https://github.com/ozla/hrtester.git
cd hrtester
```

Build the project:
```sh
go build .
```

## Examples

### HTTP

Starting servers:

```pwsh
$ # Start the mock service to simulate responses
$ $params = @("mock", "--log", ".\log\mock.log")
$ .\hrtester.exe @params &

$ # Start the collector to aggregate test results
$ $params = @("collect", "--csv", ".\results\results.csv", "--port", "51251", "--log", ".\log\collect.log")
$ .\hrtester.exe @params &

$ # Start the tester to send HTTP requests
$ $params = @("test", "--target", ":51250", "--collector", ":51251", "--port", "10090", "--log", ".\log\test.log")
$ .\hrtester  @params &
```

Starting services:

```pwsh
$ # Check the status of the mock service
$ Invoke-RestMethod -Uri "http://localhost:51250/__service" -Method Get

status
------
ready

$ # Configure the mock service with response delays
$ $data = @{
    duration = "5m"
    response = @{
        headerLatency = @{
            min = "10ms"
            max = "50ms"
        }
        duration      = @{
            min = "30ms"
            max = "100ms"
        }
    }
} | ConvertTo-Json
$ Invoke-RestMethod -Uri "http://localhost:51250/__mock" -Method Post  -ContentType "application/json" -Body $data

$ # Verify the mock service is running
$ Invoke-RestMethod -Uri "http://localhost:51250/__service" -Method Get

status  duration
------  --------
running 292363ms

$ # Configure and start a test run
$ $data = @{
>>     name            = "10s-20rps-2t"
>>     duration        = "10s"
>>     pace            = "20rps"
>>     parallelTesters = 2
>>     timeout         = "200ms"
>>     choice          = "roundrobin"
>>     reqSchema       = "http"
>>     reqIDHeader     = "X-Request-ID"
>>     requests        = @(
>>         @{
>>             method = "GET"
>>             path   = "/1"
>>             header = @{
>>                 Connection = "keep-alive"
>>             }
>>         }
>>     )
>> } | ConvertTo-Json -Depth 3
$ Invoke-RestMethod -Uri "http://localhost:10090/test" -Method Post -ContentType "application/json" -Body $data

$ # Check test service status
$ Invoke-RestMethod -Uri "http://localhost:10090/__service" -Method Get

status  duration
------  --------
testing 1262ms
```

### HTTPS (TLS-Enabled)

Starting servers:

```pwsh
$ # Start the mock service with TLS enabled
$ $params = @("mock", "--cert", "mock-cert.pem", "--key", "mock-key.pem", "--cas", "cas.pem", "--log", ".\log\mock.log")
$ .\hrtester.exe @params &

$ # Start the collector
$ $params = @("collect", "--csv", "..\results\results.csv", "--port", "51251", "--log", ".\log\collect.log")
$ .\hrtester.exe @params &

$ # Start the tester with TLS authentication
$ $params = @(
>>     "test"
>>     "--target"
>>     ":51250"
>>     "--collector"
>>     ":51251"
>>     "--port"
>>     "10090"
>>     "--cas"
>>     ".\cas.pem"
>>     "--skip-name-check"
>>     "--cert"
>>     ".\tester-cert.pem"
>>     "--key"
>>     ".\tester-key.pem"
>>     "--log"
>>     ".\log\test.log"
>> )
$ .\hrtester  @params &
```

Starting services:

```pwsh
$ # Because the --cas option is specified at mock service startup, a client certificate is required
$ $cert = Get-ChildItem -Path Cert:\CurrentUser\My\ | Where-Object -Property FriendlyName -EQ "hrtester-client"

$ # Check the status of the mock service over HTTPS
$ Invoke-RestMethod -Uri "https://localhost:51250/__service" -Method Get -SkipCertificateCheck -Certificate $cert

status
------
ready

$ # Configure the mock service with response delays
$ $data = @{
    duration = "5m"
    response = @{
        headerLatency = @{
            min = "10ms"
            max = "50ms"
        }
        duration      = @{
            min = "30ms"
            max = "100ms"
        }
    }
} | ConvertTo-Json
$ Invoke-RestMethod -Uri "https://localhost:51250/__mock" -Method Post  -ContentType "application/json" -Body $data -SkipCertificateCheck -Certificate $cert

$ # Verify the mock service is running
$ Invoke-RestMethod -Uri "https://localhost:51250/__service" -Method Get -SkipCertificateCheck -Certificate $cert

status  duration
------  --------
running 283175ms

$ # Configure and start a test run
$ $data = @{
>>     name            = "10s-20rps-2t"
>>     duration        = "10s"
>>     pace            = "20rps"
>>     parallelTesters = 2
>>     timeout         = "200ms"
>>     choice          = "roundrobin"
>>     reqSchema       = "https"
>>     reqIDHeader     = "X-Request-ID"
>>     requests        = @(
>>         @{
>>             method = "GET"
>>             path   = "/1"
>>             header = @{
>>                 Connection = "keep-alive"
>>             }
>>         }
>>     )
>> } | ConvertTo-Json -Depth 3
$ Invoke-RestMethod -Uri "https://localhost:10090/test" -Method Post -ContentType "application/json" -Body $data

$ # Check test service status
$ Invoke-RestMethod -Uri "https://localhost:10090/__service" -Method Get

status  duration
------  --------
testing 1572ms
```

## Usage and Permissions
This project is provided as-is, without warranty. You are free to use, modify, and distribute it for any purpose.
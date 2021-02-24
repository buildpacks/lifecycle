FROM golang:1.15 as builder

COPY exec.d/ /go/src/exec.d
RUN go build -o helper.exe exec.d


FROM mcr.microsoft.com/windows/nanoserver:1809

COPY container /

COPY --from=builder /gopath/helper.exe /layers/0.5_buildpack/some_layer/exec.d/helper.exe
COPY --from=builder /gopath/helper.exe /layers/0.5_buildpack/some_layer/exec.d/exec.d-checker/helper.exe

ENV PATH="c:\cnb\process;c:\cnb\lifecycle;C:\Windows\system32;C:\Windows;"

ENTRYPOINT ["c:\\cnb\\lifecycle\\launcher"]

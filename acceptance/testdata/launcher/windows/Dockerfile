FROM mcr.microsoft.com/windows/servercore:ltsc2019

COPY container /

ENV PATH="c:\cnb\process;c:\cnb\lifecycle;C:\Windows\system32;C:\Windows;"

ENTRYPOINT ["c:\\cnb\\lifecycle\\launcher"]

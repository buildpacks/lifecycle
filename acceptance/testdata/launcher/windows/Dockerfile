FROM mcr.microsoft.com/windows/nanoserver:1809

COPY container /

ENV PATH="c:\cnb\process;c:\cnb\lifecycle;C:\Windows\system32;C:\Windows;"

ENTRYPOINT ["c:\\cnb\\lifecycle\\launcher"]

@echo off

echo "hello" > file-to-snapshot
mkdir -p bin
echo "hello" > bin/exe-to-snapshot.bat

exit /b 0

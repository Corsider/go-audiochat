# go-audiochat
___
Простой full-duplex UDP голосовой чат-клиент для двух ПК.

___
# Сборка (Windows)
___
1. Установить `vcpkg`
2. Установить с помощью `vcpkg` пакеты `opusfile`, `portaudio`:
```
vcpkg install opusfile --triplet x64-mingw-static
vcpkg install portaudio --triplet x64-mingw-static 
```
3. Настроить переменные окружения Go для работы с полученными библиотеками:
```
set CGO_CFLAGS=-I{{path_to_vcpkg}}/installed/x64-mingw-static/include
set CGO_LDFLAGS=-L{{path_to_vcpkg}}}/installed/x64-mingw-static/lib -lopus -lportaudio -lwinmm -lole32 -lsetupapi
```
4. Установить путь до PRG_CONFIG пакетов:
```
$env:PKG_CONFIG_PATH = "{{path_to_vcpkg}}/installed/x64-mingw-static/lib/pkgconfig"
```
5. `go mod download`, `go build`

# Запуск (пример)
___
ПК №1 (адрес 192.168.0.120):
```
.\go-audiochat.exe -d 192.168.0.100:9001 -l :9000
```

ПК №2 (адрес 192.168.0.100):
```
.\go-audiochat.exe -d 192.168.0.120:9000 -l :9001
```
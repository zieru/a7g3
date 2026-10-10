@echo off
setlocal enabledelayedexpansion

echo ========================================================
echo   a7g3 Auto Commit, Version Increment, and Push
echo ========================================================

:: 1. Git add semua perubahan
echo.
echo [1/4] Menambahkan file yang berubah (git add .)...
git add .

:: 2. Meminta pesan commit
echo.
set /p commit_msg="Masukkan pesan commit (tekan Enter untuk auto-generate): "
if "%commit_msg%"=="" (
    set commit_msg=Auto commit: %date% %time%
)

echo.
echo [2/4] Melakukan commit...
git commit -m "!commit_msg!"

:: 3. Memilih Opsi Rilis
echo.
echo ========================================================
echo   PILIH TARGET RILIS GITHUB ACTIONS:
echo   [1] Release Linux Only (Cepat, rilis g3a-linux-amd64)
echo   [2] Release All (Linux amd64 + Windows amd64)
echo   [3] Commit dan Push saja (Tanpa rilis tag)
echo ========================================================
set /p release_choice="Pilihan Anda [1/2/3] (default: 1): "
if "%release_choice%"=="" set release_choice=1

if "%release_choice%"=="3" (
    echo.
    echo [3/4] Melewati pembuatan tag rilis...
    echo.
    echo [4/4] Mendorong kode ke origin main...
    git push origin HEAD
    goto :selesai
)

set SUFFIX_ARG=
if "%release_choice%"=="1" (
    set SUFFIX_ARG=-Suffix linux
)

echo.
echo [3/4] Mengambil versi tag terakhir dari remote dan lokal...
set CURRENT_TAG=
set NEW_TAG=

for /f "tokens=1,2" %%A in ('powershell -NoProfile -ExecutionPolicy Bypass -File scripts\get_next_tag.ps1 %SUFFIX_ARG%') do (
    set CURRENT_TAG=%%A
    set NEW_TAG=%%B
)

if "!NEW_TAG!"=="" (
    set CURRENT_TAG=v1.0.10
    if "%release_choice%"=="1" (
        set NEW_TAG=v1.0.11-linux
    ) else (
        set NEW_TAG=v1.0.11
    )
)

echo.
echo Tag tertinggi saat ini : !CURRENT_TAG!
echo Tag baru yang dibuat   : !NEW_TAG!

git tag !NEW_TAG!

echo.
echo [4/4] Mendorong kode dan tag (!NEW_TAG!) ke GitHub...
git push origin HEAD
git push origin !NEW_TAG!

:selesai
echo.
echo ========================================================
if "%release_choice%"=="3" (
    echo Selesai: Kode berhasil di-push ke main tanpa tag.
) else (
    echo Selesai: Kode dan tag (!NEW_TAG!) berhasil di-push!
    echo GitHub Actions sedang memproses Build ^& Release.
)
echo ========================================================
pause
endlocal

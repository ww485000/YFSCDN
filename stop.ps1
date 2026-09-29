# EdgeCDN stop: kill local core.exe / edge.exe instances.
param([switch]$All) # -All also kills the dummy test origin (node origin.js)

Get-Process core, edge -ErrorAction SilentlyContinue | ForEach-Object {
    Write-Host "stopping $($_.Name) ($($_.Id))"
    Stop-Process -Id $_.Id -Force
}
if ($All) {
    Get-CimInstance Win32_Process -Filter "Name = 'node.exe'" | Where-Object {
        $_.CommandLine -match 'origin\.js|smoke\.js'
    } | ForEach-Object {
        Write-Host "stopping test helper (pid $($_.ProcessId))"
        Stop-Process -Id $_.ProcessId -Force
    }
    Get-CimInstance Win32_Process -Filter "Name = 'pwsh.exe' OR Name = 'powershell.exe'" | Where-Object {
        $_.CommandLine -match 'origin-test\.ps1|tcp-echo\.ps1'
    } | ForEach-Object {
        Write-Host "stopping test origin (pid $($_.ProcessId))"
        Stop-Process -Id $_.ProcessId -Force
    }
}
Write-Host "done."

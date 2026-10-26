param(
    [Parameter(Mandatory=$true)][int]$ProcessId,
    [ValidateRange(2,60)][int]$Seconds=20,
    [string]$Phase='listening'
)
$ErrorActionPreference='Stop'
$initial=Get-Process -Id $ProcessId
$cpuStart=$initial.TotalProcessorTime.TotalSeconds
$clock=[Diagnostics.Stopwatch]::StartNew()
Start-Sleep -Seconds $Seconds
$final=Get-Process -Id $ProcessId
$cpu=100*($final.TotalProcessorTime.TotalSeconds-$cpuStart)/$clock.Elapsed.TotalSeconds
[pscustomobject]@{
    Phase=$Phase
    Seconds=[math]::Round($clock.Elapsed.TotalSeconds,2)
    CPUOneCorePercent=[math]::Round($cpu,3)
    CPUMachinePercent=[math]::Round($cpu/[Environment]::ProcessorCount,3)
    WorkingSetMiB=[math]::Round($final.WorkingSet64/1MB,1)
    PrivateBytesMiB=[math]::Round($final.PrivateMemorySize64/1MB,1)
    Handles=$final.HandleCount
    Threads=$final.Threads.Count
}

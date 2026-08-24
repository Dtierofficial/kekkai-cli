$ErrorActionPreference = 'Stop'

$repo = 'Dtierofficial/kekkai-cli'
$installDir = Join-Path $HOME '.kekkai\bin'
$apiHeaders = @{ 'User-Agent' = 'kekkai-installer'; 'Accept' = 'application/vnd.github+json' }

if ([Environment]::Is64BitOperatingSystem -eq $false) {
    throw 'Kekkai requires a 64-bit Windows installation.'
}

$architecture = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
switch ($architecture.ToUpperInvariant()) {
    'AMD64' { $arch = 'amd64' }
    'ARM64' { $arch = 'arm64' }
    default { throw "Unsupported Windows architecture: $architecture" }
}

$release = Invoke-RestMethod -Uri "https://api.github.com/repos/$repo/releases/latest" -Headers $apiHeaders
$assetNames = @("kekkai-windows-$arch.exe", 'kekkai.exe')
$asset = $null
foreach ($name in $assetNames) {
    $asset = $release.assets | Where-Object { $_.name -eq $name } | Select-Object -First 1
    if ($null -ne $asset) { break }
}
if ($null -eq $asset) {
    throw "No Windows $arch binary was found in release $($release.tag_name). Expected: $($assetNames -join ', ')"
}

New-Item -ItemType Directory -Path $installDir -Force | Out-Null
$target = Join-Path $installDir 'kekkai.exe'
Invoke-WebRequest -Uri $asset.browser_download_url -Headers $apiHeaders -OutFile $target

# Verify the binary against the release checksum manifest (SHA256SUMS.txt).
# A manifest that cannot be fetched only produces a warning; a fetched
# manifest that does not match is a hard failure.
$sumsUrl = "https://github.com/$repo/releases/latest/download/SHA256SUMS.txt"
try {
    $sums = (Invoke-WebRequest -Uri $sumsUrl -Headers $apiHeaders -UseBasicParsing).Content
    $line = ($sums -split "`n") | Where-Object { $_ -match [regex]::Escape($asset.name) } | Select-Object -First 1
    if ($line) {
        $expected = $line.Trim() -split '\s+' | Select-Object -First 1
        $actual = (Get-FileHash -Path $target -Algorithm SHA256).Hash.ToLower()
        if ($actual -ne $expected.ToLower()) {
            Remove-Item $target -Force
            throw 'SHA256 checksum mismatch: the downloaded binary is corrupted or has been tampered with.'
        }
        Write-Host 'Checksum verified.'
    } else {
        Write-Warning 'Checksum manifest did not contain this asset; skipping verification.'
    }
} catch [System.Management.Automation.RuntimeException] {
    throw
} catch {
    Write-Warning "Could not fetch SHA256SUMS.txt; skipping checksum verification."
}

$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
$pathParts = @($userPath -split ';' | Where-Object { $_ -ne '' })
if (-not ($pathParts | Where-Object { $_.TrimEnd('\') -ieq $installDir.TrimEnd('\') })) {
    $newPath = if ([string]::IsNullOrWhiteSpace($userPath)) { $installDir } else { "$userPath;$installDir" }
    [Environment]::SetEnvironmentVariable('Path', $newPath, 'User')
}

if (-not (($env:Path -split ';' | Where-Object { $_.TrimEnd('\') -ieq $installDir.TrimEnd('\') }))) {
    $env:Path = "$installDir;$env:Path"
}

Write-Host "Kekkai successfully installed! Type 'kekkai' to get started."

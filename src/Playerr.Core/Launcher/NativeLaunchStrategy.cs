using System;
using System.Diagnostics;
using System.IO;
using System.Runtime.InteropServices;
using System.Threading.Tasks;
using Playerr.Core.Games;

namespace Playerr.Core.Launcher
{
    public class NativeLaunchStrategy : ILaunchStrategy
    {
        public bool IsSupported(Game game)
        {
            // Support if we have an explicit ExecutablePath file
            return !string.IsNullOrEmpty(game.ExecutablePath) && File.Exists(game.ExecutablePath);
        }

        public Task LaunchAsync(Game game, string? overridePath = null)
        {
            if (string.IsNullOrEmpty(game.ExecutablePath))
            {
                throw new InvalidOperationException("Game executable path is not set.");
            }

            var path = !string.IsNullOrEmpty(overridePath) ? overridePath : game.ExecutablePath;
            if (string.IsNullOrEmpty(path)) throw new InvalidOperationException("No executable path provided.");
            var directory = Path.GetDirectoryName(path);
            
            System.Console.WriteLine($"[NativeLaunchStrategy] Launching: {path}");

            var startInfo = new ProcessStartInfo();
            startInfo.WorkingDirectory = directory;

            if (RuntimeInformation.IsOSPlatform(OSPlatform.Windows))
            {
                startInfo.FileName = path;
                startInfo.UseShellExecute = true; 
            }
            else if (RuntimeInformation.IsOSPlatform(OSPlatform.OSX))
            {
                // Must use 'open' -W -n to wait/new instance or just plain open on the app bundle if it is one.
                // If it is a Unix executable inside a .app, launching directly might fail if it expects bundle context.
                // Safe bet: 'open "path"'
                startInfo.FileName = "open";
                startInfo.Arguments = $"\"{path}\"";
                startInfo.UseShellExecute = false; 
            }
            else
            {
                // Linux logic
                // Check if it's a Windows .exe -> Wine
                if (Path.GetExtension(path).Equals(".exe", StringComparison.OrdinalIgnoreCase))
                {
                    startInfo.FileName = "wine";
                    startInfo.Arguments = $"\"{path}\"";
                }
                else
                {
                    // Native binary
                    startInfo.FileName = path;
                }
                startInfo.UseShellExecute = false;
            }

            try
            {
                Process.Start(startInfo);
            }
            catch (Exception ex)
            {
                System.Console.WriteLine($"[NativeLaunchStrategy] Error: {ex.Message}");
                throw;
            }

            return Task.CompletedTask;
        }
    }
}

using System;
using System.Diagnostics;
using System.IO;
using System.Linq;
using System.Runtime.InteropServices;
using System.Threading.Tasks;
using Playerr.Core.Games;

namespace Playerr.Core.Launcher
{
    public class NativeLaunchStrategy : ILaunchStrategy
    {
        public bool IsSupported(Game game)
        {
            if (!string.IsNullOrEmpty(game.ExecutablePath) && File.Exists(game.ExecutablePath))
                return true;

            var dirPath = !string.IsNullOrEmpty(game.ExecutablePath) ? game.ExecutablePath : game.Path;
            if (!string.IsNullOrEmpty(dirPath) && Directory.Exists(dirPath))
                return FindExecutable(dirPath) != null;

            return false;
        }

        private static string? FindExecutable(string directory)
        {
            var skipPatterns = new[] { "redist", "setup", "install", "uninstall", "crash", "dxsetup", "vcredist", "directx" };

            var exes = Directory.GetFiles(directory, "*.exe", SearchOption.TopDirectoryOnly);
            if (exes.Length == 0)
                exes = Directory.GetFiles(directory, "*.exe", SearchOption.AllDirectories);

            var filtered = exes.Where(e => !skipPatterns.Any(p =>
                Path.GetFileNameWithoutExtension(e).Contains(p, StringComparison.OrdinalIgnoreCase))).ToArray();

            if (filtered.Length > 0) exes = filtered;

            return exes.OrderByDescending(f => new FileInfo(f).Length).FirstOrDefault();
        }

        public Task LaunchAsync(Game game, string? overridePath = null)
        {
            var path = !string.IsNullOrEmpty(overridePath) ? overridePath
                     : !string.IsNullOrEmpty(game.ExecutablePath) ? game.ExecutablePath
                     : game.Path;

            if (string.IsNullOrEmpty(path)) throw new InvalidOperationException("No executable path provided.");

            // Resolve directory to actual executable
            if (Directory.Exists(path) && !File.Exists(path))
            {
                var resolved = FindExecutable(path);
                if (resolved == null)
                    throw new InvalidOperationException($"Could not find an executable in: {path}");
                System.Console.WriteLine($"[NativeLaunchStrategy] Resolved directory to executable: {resolved}");
                path = resolved;
            }

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
                    startInfo.Environment["WINEDEBUG"] = "-all";
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

using Microsoft.AspNetCore.Builder;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Hosting;
using Microsoft.Extensions.FileProviders;
using Microsoft.AspNetCore.StaticFiles;
using Microsoft.AspNetCore.StaticFiles;
using Microsoft.AspNetCore.Server.Kestrel.Core;
using Microsoft.AspNetCore.Hosting;
using Microsoft.AspNetCore.Http;
using System;
using System.IO;
using Playerr.Core.Games;
using Playerr.Core.MetadataSource;
using Playerr.Core.MetadataSource.Steam;
using Playerr.Core.MetadataSource.Igdb;
using Playerr.Core.Prowlarr;
using Playerr.Core.Jackett;
using Playerr.Core.Configuration;
using System.Linq;
using Photino.NET;

namespace Playerr.Host
{
    public class Program
    {
        [STAThread]
        public static void Main(string[] args)
        {
            var builder = WebApplication.CreateBuilder(args);

            // Add services
            builder.Services.AddControllers()
                .AddApplicationPart(typeof(Playerr.Api.V3.Games.GameController).Assembly)
                .AddApplicationPart(typeof(Playerr.Api.V3.Settings.MediaController).Assembly);
            
            builder.Services.AddEndpointsApiExplorer();
            builder.Services.AddSwaggerGen();

            // Add CORS for development
            builder.Services.AddCors(options =>
            {
                options.AddDefaultPolicy(policy =>
                {
                    policy.AllowAnyOrigin()
                          .AllowAnyMethod()
                          .AllowAnyHeader();
                });
            });

            // Configuration service for persistence - Use AppContext.BaseDirectory to look next to the executable
            var exePath = AppContext.BaseDirectory;
            var contentRoot = exePath; // Default content root to exe location for portable apps
            
            // Allow override via args or env if needed, but default to portable behavior
            // Note: builder.Environment.ContentRootPath defaults to CWD, which is bad for portable apps ran from elsewhere
            
            var configService = new ConfigurationService(contentRoot);
            builder.Services.AddSingleton(configService);

            // IGDB metadata services - use factory pattern for dynamic configuration
            builder.Services.AddSingleton<IGameMetadataServiceFactory, GameMetadataServiceFactory>();
            builder.Services.AddSingleton<IGameRepository, InMemoryGameRepository>();
            builder.Services.AddSingleton<MediaScannerService>();
            
            // IO Services
            builder.Services.AddSingleton<Playerr.Core.IO.IFileMoverService, Playerr.Core.IO.FileMoverService>();

            // Register SteamClient for direct usage (e.g. Settings Test/Sync)
            builder.Services.AddTransient<SteamClient>();
            
            
            // Show IGDB status at startup
            var igdbSettings = configService.LoadIgdbSettings();
            if (!igdbSettings.IsConfigured)
            {
                Console.WriteLine("WARNING: IGDB credentials not configured. Game search will return 0 results. Configure via Settings API.");
            }
            else
            {
                Console.WriteLine("IGDB credentials loaded from persistent configuration.");
            }
            
            // Configure Prowlarr settings - load from persistent config
            var prowlarrSettings = configService.LoadProwlarrSettings();
            builder.Services.AddSingleton(prowlarrSettings);

            // Configure Jackett settings - load from persistent config
            var jackettSettings = configService.LoadJackettSettings();
            builder.Services.AddSingleton(jackettSettings);

            // Configure Kestrel to use a dynamic port (0) to avoid conflicts (Address already in use)
            // This is crucial for desktop apps where we can't guarantee a specific port is free
            // CHECK IF RUNNING IN CONTAINER OR HEADLESS
            var envVar = Environment.GetEnvironmentVariable("DOTNET_RUNNING_IN_CONTAINER");
            var isHeadless = args.Contains("--headless") || envVar == "true";
            
            Console.WriteLine($"[Startup] Checking Headless Mode: Args={string.Join(",", args)}, EnvVar={envVar}, Result={isHeadless}");

            if (!isHeadless)
            {
                builder.WebHost.ConfigureKestrel(serverOptions =>
                {
                    serverOptions.Listen(System.Net.IPAddress.Loopback, 5002);
                });
            }
            // ELSE: Let Kestrel use default config (ASPNETCORE_URLS) which is ideal for Docker

            var app = builder.Build();

            // Configure middleware
            if (app.Environment.IsDevelopment())
            {
                app.UseSwagger();
                app.UseSwaggerUI();
            }

            app.UseCors();
            
            // Configure static files - Look for _output/UI relative to the EXECUTABLE
            // In dev: AppContext.BaseDirectory is usually bin/Debug/net8.0/
            // In prod (single file): AppContext.BaseDirectory is where the .exe is.
            
            // Try to find the UI folder. 
            // 1. Production: ./_output/UI (next to exe)
            // 2. Dev: ../../../../../_output/UI (relative to bin debug)
            
            var uiPath = Path.Combine(exePath, "_output", "UI");
            
            // Search strategy for UI folder:
            // 1. Next to exe in _output/UI 
            // 2. One level up in UI (if bin is in _output/net8.0/)
            // 3. Dev environment (5 levels up from bin/Debug/net8.0/...)
            
            if (!Directory.Exists(uiPath))
            {
                 // Try one level up (common if exe is in _output/net8.0/ and UI is in _output/UI)
                 var parentPath = Path.GetFullPath(Path.Combine(exePath, "..", "UI"));
                 if (Directory.Exists(parentPath))
                 {
                     uiPath = parentPath;
                 }
                 else
                 {
                     // Fallback for development if not found next to dll
                     var potentialDevPath = Path.GetFullPath(Path.Combine(exePath, "..", "..", "..", "..", "..", "_output", "UI"));
                     if (Directory.Exists(potentialDevPath))
                     {
                         uiPath = potentialDevPath;
                     }
                 }
            }
            

            
            if (Directory.Exists(uiPath))
            {
                var fileProvider = new PhysicalFileProvider(uiPath);
                
                var defaultFilesOptions = new DefaultFilesOptions
                {
                    FileProvider = fileProvider
                };
                defaultFilesOptions.DefaultFileNames.Clear();
                defaultFilesOptions.DefaultFileNames.Add("index.html");
                app.UseDefaultFiles(defaultFilesOptions);
                
                app.UseStaticFiles(new StaticFileOptions
                {
                    FileProvider = fileProvider
                });
            }
            else
            {
                Console.WriteLine($"WARNING: UI directory not found at {uiPath}. Ensure _output/UI is copied next to the executable.");
            }
            
            app.UseRouting();
            app.UseAuthorization();
            app.MapControllers();

            // Serve frontend - fallback to index.html for SPA routing
            if (Directory.Exists(uiPath))
            {
                app.MapFallback(async context =>
                {
                    var indexPath = Path.Combine(uiPath, "index.html");
                    var html = await File.ReadAllTextAsync(indexPath);
                    context.Response.ContentType = "text/html";
                    await context.Response.Body.WriteAsync(System.Text.Encoding.UTF8.GetBytes(html));
                });
            }

            // Helper to open browser
            Action<string> OpenBrowser = (url) => 
            {
                try
                {
                    if (System.Runtime.InteropServices.RuntimeInformation.IsOSPlatform(System.Runtime.InteropServices.OSPlatform.Windows))
                    {
                        System.Diagnostics.Process.Start(new System.Diagnostics.ProcessStartInfo("cmd", $"/c start {url}") { CreateNoWindow = true });
                    }
                    else if (System.Runtime.InteropServices.RuntimeInformation.IsOSPlatform(System.Runtime.InteropServices.OSPlatform.Linux))
                    {
                        System.Diagnostics.Process.Start("xdg-open", url);
                    }
                    else if (System.Runtime.InteropServices.RuntimeInformation.IsOSPlatform(System.Runtime.InteropServices.OSPlatform.OSX))
                    {
                        System.Diagnostics.Process.Start("open", url);
                    }
                }
                catch (Exception ex)
                {
                    Console.WriteLine($"Failed to open browser: {ex.Message}");
                }
            };

            // Kestrel is configured via appsettings or LaunchSettings to listen on 5001
            // We need to start the app non-blocking
            app.Start();

            // Get the actual address assigned by Kestrel (since we used port 0)
            // We need to request the IServer interface to get the features
            var server = app.Services.GetRequiredService<Microsoft.AspNetCore.Hosting.Server.IServer>();
            var addressFeature = server.Features.Get<Microsoft.AspNetCore.Hosting.Server.Features.IServerAddressesFeature>();
            
            var address = addressFeature?.Addresses.FirstOrDefault() ?? "http://localhost:5001";
            
            Console.WriteLine($"Playerr running on {address}");
            
            Console.WriteLine($"Playerr running on {address}");
            
            if (isHeadless)
            {
                 Console.WriteLine("Running in Headless Mode (Docker/Server). Press Ctrl+C to exit.");
                 app.WaitForShutdown();
            }
            else
            {
                // Launch Photino Window
                // This blocks until the window is closed
                try 
                {
                   var window = new Photino.NET.PhotinoWindow()
                       .SetTitle("Playerr")
                       .SetUseOsDefaultSize(false)
                       .SetSize(new System.Drawing.Size(1280, 800))
                       .Center()
                       .SetResizable(true);
    
                   // Real-time library updates: Subscribe to scanner events
                   var scannerService = app.Services.GetRequiredService<MediaScannerService>();
                   
                   // Update library UI when a batch is finished
                   scannerService.OnBatchFinished += () => {
                       Console.WriteLine("[UI] Sending LIBRARY_UPDATED signal to frontend...");
                       window.SendWebMessage("LIBRARY_UPDATED");
                   };
    
                   window.RegisterWebMessageReceivedHandler((object sender, string message) => {
                           var window = (Photino.NET.PhotinoWindow)sender;
    
                           // Handle messages from frontend
                           if (message.StartsWith("OPEN_URL:"))
                           {
                               var url = message.Substring("OPEN_URL:".Length);
                               OpenBrowser(url);
                           }
                           else if (message == "SELECT_FOLDER")
                           {
                               var folders = window.ShowOpenFolder();
                               if (folders != null && folders.Length > 0)
                               {
                                   var selectedPath = folders[0];
                                   // Direct Save to Backend Configuration
                                   // Resolving service from app.Services (which we need access to here, but scope is tricky)
                                   // We need capturing 'app' variable or 'configService' variable into this closure.
                                   // Fortunately, this lambda captures local variables of Main method.
                                   
                                   try 
                                   {
                                        var currentMediaSettings = configService.LoadMediaSettings();
                                        currentMediaSettings.FolderPath = selectedPath;
                                        configService.SaveMediaSettings(currentMediaSettings);
                                        
                                        // 1. Notify UI with specific path for immediate feedback
                                        window.SendWebMessage($"FOLDER_SELECTED:{selectedPath}");
    
                                        // 2. Notify UI that settings have changed (reload for consistency)
                                        window.SendWebMessage("SETTINGS_UPDATED");
                                   }
                                   catch (Exception ex)
                                   {
                                        Console.WriteLine($"Error saving folder selection: {ex.Message}");
                                        // Fallback: try to send path anyway for debug
                                        window.SendWebMessage($"FOLDER_SELECTED:{selectedPath}");
                                   }
                               }
                           }
                       })
                       .Load(address + "?v=" + DateTime.Now.Ticks);
                       
                    window.WaitForClose(); // Blocks main thread
                }
                catch (Exception ex)
                {
                    Console.WriteLine($"Failed to launch Photino Window: {ex.Message}");
                    // If native window fails (e.g. headless linux), keep running as console
                    Console.WriteLine("Running in Console mode (Server only). Press Ctrl+C to exit.");
                    app.WaitForShutdown(); 
                }
                finally
                {
                    // Ensure the app shuts down completely when the window is closed
                    Console.WriteLine("Window closed. Shutting down application...");
                    
                    // Graceful shutdown of Kestrel
                    try 
                    {
                        app.StopAsync().GetAwaiter().GetResult();
                        app.DisposeAsync().GetAwaiter().GetResult();
                    }
                    catch (Exception shutdownEx)
                    {
                        Console.WriteLine($"Error during shutdown: {shutdownEx.Message}");
                    }
    
                    // Force process exit to ensure no background threads (like Kestrel) keep the process alive
                    Environment.Exit(0);
                }
            }
        }
    }
}

using Microsoft.AspNetCore.Builder;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Hosting;
using Microsoft.EntityFrameworkCore;
using Playerr.Core.Data;
using Microsoft.Extensions.FileProviders;
using Microsoft.AspNetCore.StaticFiles;
using Microsoft.AspNetCore.Server.Kestrel.Core;
using Microsoft.AspNetCore.Hosting;
using Microsoft.AspNetCore.Http;
using System;
using System.Threading.Tasks;
using System.IO;
using Playerr.Core.Games;
using Playerr.Core.MetadataSource;
using Playerr.Core.MetadataSource.Steam;
using Playerr.Core.MetadataSource.Igdb;
using Playerr.Core.Download;
using Playerr.Core.Prowlarr;
using Playerr.Core.Jackett;
using Playerr.Core.Configuration;
using System.Linq;
using Photino.NET;
using System.Diagnostics.CodeAnalysis;

namespace Playerr.Host
{
    [SuppressMessage("Microsoft.Design", "CA1052:StaticHolderTypesShouldBeSealed")]
    [SuppressMessage("Microsoft.Design", "CA1031:DoNotCatchGeneralExceptionTypes")]
    [SuppressMessage("Microsoft.Reliability", "CA2007:DoNotDirectlyAwaitATask")]
    [SuppressMessage("Microsoft.Globalization", "CA1303:Do not pass literals as localized parameters")]
    [SuppressMessage("Microsoft.Globalization", "CA1310:SpecifyStringComparison")]
    [SuppressMessage("Microsoft.Usage", "CA2012:UseValueTasksCorrectly")]
    public class Program
    {
        [STAThread]
        public static async Task Main(string[] args)
        {
            var exePath = AppContext.BaseDirectory;
            var builder = WebApplication.CreateBuilder(new WebApplicationOptions
            {
                Args = args,
                ContentRootPath = exePath
            });
            // Add services
            System.Console.WriteLine("DEBUG: Registering Services...");
            builder.Services.AddControllers()
                .AddApplicationPart(typeof(Playerr.Api.V3.Games.GameController).Assembly)
                .AddApplicationPart(typeof(Playerr.Api.V3.Settings.MediaController).Assembly)
                .AddApplicationPart(typeof(Playerr.Api.V3.Settings.HydraController).Assembly);
            
            builder.Services.AddEndpointsApiExplorer();
            builder.Services.AddSwaggerGen();
            builder.Services.AddHttpClient(); // Register IHttpClientFactory

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

            // Configuration service for persistence
            var configPath = Path.Combine(exePath, "config");
            
            // In development/build scenarios, the exe is deep in _output/net8.0/osx-arm64/
            // We want to look for the 'config' folder in the project root so it persists across builds (which wipe _output)
            if (!Directory.Exists(configPath))
            {
                // Try to find project root by looking for the 'config' folder up the tree
                var candidate = exePath;
                bool found = false;

                // 1. Try relative search (works for Terminal runs)
                for (int i = 0; i < 10; i++)
                {
                    candidate = Path.GetDirectoryName(candidate);
                    if (candidate == null) break;
                    
                    var checkPath = Path.Combine(candidate, "config");
                    if (Directory.Exists(checkPath))
                    {
                        Console.WriteLine($"[Config] Found persistent configuration at: {checkPath}");
                        configPath = checkPath;
                        exePath = candidate; 
                        found = true;
                        break;
                    }
                }

                // 2. Fallback for macOS App Translocation / Sandbox (works for .app double-click)
                if (!found)
                {
                     // Use standard ApplicationData folder as ultimate fallback for config
                     var appData = Environment.GetFolderPath(Environment.SpecialFolder.ApplicationData);
                     var appDataConfig = Path.Combine(appData, "Playerr", "config");
                     
                     if (Directory.Exists(appDataConfig))
                     {
                         Console.WriteLine($"[Config] Using AppData configuration at: {appDataConfig}");
                         configPath = appDataConfig;
                         // exePath remains where the executable is
                     }
                }
            }
            
            // Note: ConfigurationService adds "/config" to the path passed to it
            var configService = new ConfigurationService(exePath);
            builder.Services.AddSingleton(configService);

            // Persistence with SQLite
            var dbPath = Path.Combine(configPath, "playerr.db");
            builder.Services.AddDbContextFactory<PlayerrDbContext>(options =>
                options.UseSqlite($"Data Source={dbPath}"));

            builder.Services.AddSingleton<IGameRepository, SqliteGameRepository>();
            builder.Services.AddSingleton<IGameMetadataServiceFactory, GameMetadataServiceFactory>();
            builder.Services.AddSingleton<MediaScannerService>();
            
            // IO Services
            builder.Services.AddSingleton<Playerr.Core.IO.IFileMoverService, Playerr.Core.IO.FileMoverService>();
            builder.Services.AddSingleton<Playerr.Core.IO.IArchiveService, Playerr.Core.IO.ArchiveService>();

            // Post-Download Management
            builder.Services.AddSingleton<Playerr.Core.Download.PostDownloadProcessor>();
            builder.Services.AddSingleton<Playerr.Core.Download.DownloadMonitorService>();
            builder.Services.AddHostedService(sp => sp.GetRequiredService<Playerr.Core.Download.DownloadMonitorService>());
            builder.Services.AddSingleton<Playerr.Core.Download.ImportStatusService>();

            // Launch Services
            builder.Services.AddSingleton<Playerr.Core.Launcher.ILaunchStrategy, Playerr.Core.Launcher.SteamLaunchStrategy>();
            builder.Services.AddSingleton<Playerr.Core.Launcher.ILaunchStrategy, Playerr.Core.Launcher.NativeLaunchStrategy>();
            builder.Services.AddSingleton<Playerr.Core.Launcher.ILauncherService, Playerr.Core.Launcher.LauncherService>();
            
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
            app.UseDeveloperExceptionPage(); // FORCE DEBUG
            if (app.Environment.IsDevelopment())
            {
                app.UseSwagger();
                app.UseSwaggerUI();
            }

            // Initialize database
            using (var scope = app.Services.CreateScope())
            {
                var context = scope.ServiceProvider.GetRequiredService<PlayerrDbContext>();
                try
                {
                    context.Database.EnsureCreated();
                    Console.WriteLine($"[Database] SQLite initialized at: {dbPath}");

                    // [Schema Update] v0.4.0
                    Console.WriteLine("[Database] Checking for schema updates (v0.4.0)...");
                    try {
                        // Check if ExecutablePath column exists
                        context.Database.ExecuteSqlRaw("ALTER TABLE Games ADD COLUMN ExecutablePath TEXT;");
                        Console.WriteLine("[Schema] Added ExecutablePath column.");
                    } catch {} 

                    try {
                        // Check if IsExternal column exists
                        context.Database.ExecuteSqlRaw("ALTER TABLE Games ADD COLUMN IsExternal INTEGER DEFAULT 0;");
                         Console.WriteLine("[Schema] Added IsExternal column.");
                    } catch {}

                    // Ensure required platforms exist (Seed missing ones for existing databases)
                    Console.WriteLine("[Database] Verifying default platforms...");
                    
                    var defaultPlatforms = new[]
                    {
                        new Playerr.Core.Games.Platform { Id = 6, Name = "PC (Microsoft Windows)", Slug = "pc", Type = Playerr.Core.Games.PlatformType.PC },
                        new Playerr.Core.Games.Platform { Id = 3, Name = "Linux", Slug = "linux", Type = Playerr.Core.Games.PlatformType.PC },
                        new Playerr.Core.Games.Platform { Id = 14, Name = "Mac", Slug = "mac", Type = Playerr.Core.Games.PlatformType.MacOS },
                        new Playerr.Core.Games.Platform { Id = 7, Name = "PlayStation", Slug = "ps1", Type = Playerr.Core.Games.PlatformType.PlayStation },
                        new Playerr.Core.Games.Platform { Id = 8, Name = "PlayStation 2", Slug = "ps2", Type = Playerr.Core.Games.PlatformType.PlayStation2 },
                        new Playerr.Core.Games.Platform { Id = 9, Name = "PlayStation 3", Slug = "ps3", Type = Playerr.Core.Games.PlatformType.PlayStation3 },
                        new Playerr.Core.Games.Platform { Id = 48, Name = "PlayStation 4", Slug = "ps4", Type = Playerr.Core.Games.PlatformType.PlayStation4 },
                        new Playerr.Core.Games.Platform { Id = 130, Name = "Nintendo Switch", Slug = "switch", Type = Playerr.Core.Games.PlatformType.Switch },
                        new Playerr.Core.Games.Platform { Id = 167, Name = "PlayStation 5", Slug = "ps5", Type = Playerr.Core.Games.PlatformType.PlayStation5 },
                        new Playerr.Core.Games.Platform { Id = 169, Name = "Xbox Series X|S", Slug = "xbox-series-x", Type = Playerr.Core.Games.PlatformType.XboxSeriesX },
                        new Playerr.Core.Games.Platform { Id = 38, Name = "PlayStation Portable", Slug = "psp", Type = Playerr.Core.Games.PlatformType.PSP }
                    };

                    bool changesMade = false;
                    foreach (var platform in defaultPlatforms)
                    {
                        if (!context.Platforms.Any(p => p.Id == platform.Id))
                        {
                            Console.WriteLine($"[Database] Seeding missing platform: {platform.Name} (ID: {platform.Id})");
                            context.Platforms.Add(platform);
                            changesMade = true;
                        }
                    }

                    if (changesMade)
                    {
                        context.SaveChanges();
                        Console.WriteLine("[Database] Platforms updated.");
                    }

                    // MANUAL MIGRATION for v0.3.9+ (Images Support)
                    // Since EF Core Migrations are complex to set up in this environment, 
                    // we manually ensure the new columns exist for users upgrading from < v0.3.9.
                    try 
                    {
                        Console.WriteLine("[Database] Checking for schema updates (v0.3.12)...");
                        var connection = context.Database.GetDbConnection();
                        await connection.OpenAsync();
                        using var cmd = connection.CreateCommand();
                        
                        // We use a helper to try adding columns. SQLite ignores repeated ADD COLUMN? No, it throws.
                        // So we catch exceptions per column.
                        var columns = new[] 
                        {
                            "Images_CoverUrl TEXT",
                            "Images_CoverLargeUrl TEXT",
                            "Images_BackgroundUrl TEXT",
                            "Images_BannerUrl TEXT",
                            "Images_Screenshots TEXT", // JSON
                            "Images_Artworks TEXT",    // JSON
                            "IsInstallable INTEGER NOT NULL DEFAULT 0",
                            "InstallPath TEXT",
                            "IgdbId INTEGER",
                            "SteamId INTEGER",
                            "GogId TEXT"
                        };

                        foreach (var colDef in columns)
                        {
                            try 
                            {
                                cmd.CommandText = $"ALTER TABLE Games ADD COLUMN {colDef};";
                                await cmd.ExecuteNonQueryAsync();
                                Console.WriteLine($"[Database] Configured legacy column: {colDef.Split(' ')[0]}");
                            }
                            catch (Exception)
                            {
                                // Column likely exists. Ignore.
                            }
                        }
                        
                        await connection.CloseAsync();
                    }
                    catch (Exception ex)
                    {
                        Console.WriteLine($"[Database] Schema check warning: {ex.Message}");
                    }
                }
                catch (Exception ex)
                {
                    Console.WriteLine($"[Database] Error initializing SQLite: {ex.Message}");
                }
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
                     else
                     {
                         // Fallback using CurrentDirectory (useful for dotnet run)
                         // PWD = src/Playerr.Host
                         // Target = _output/UI (in project root)
                         // Path = ../../_output/UI
                         var pwdPath = Path.GetFullPath(Path.Combine(Directory.GetCurrentDirectory(), "..", "..", "_output", "UI"));
                         if (Directory.Exists(pwdPath))
                         {
                             uiPath = pwdPath;
                         }
                     }
                 }
            }
            
            if (Directory.Exists(uiPath))
            {
                Console.WriteLine($"[UI] Serving static files from: {uiPath}");
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
    
                   // Fix for CS8622: Use object? for sender
                   window.RegisterWebMessageReceivedHandler((object? sender, string message) => {
                           if (sender is not Photino.NET.PhotinoWindow windowInstance) return;
    
                           // Handle messages from frontend
                           if (message.StartsWith("OPEN_URL:", StringComparison.OrdinalIgnoreCase))
                           {
                               var url = message.Substring("OPEN_URL:".Length);
                               OpenBrowser(url);
                           }
                           else if (message.StartsWith("SELECT_FOLDER"))
                           {
                               var folders = windowInstance.ShowOpenFolder();
                               if (folders != null && folders.Length > 0)
                               {
                                   var selectedPath = folders[0];
                                   
                                   try 
                                   {
                                        var currentMediaSettings = configService.LoadMediaSettings();
                                        
                                        if (message.Contains("DOWNLOAD"))
                                        {
                                            currentMediaSettings.DownloadPath = selectedPath;
                                        }
                                        else if (message.Contains("DESTINATION"))
                                        {
                                            currentMediaSettings.DestinationPath = selectedPath;
                                        }
                                        else
                                        {
                                            currentMediaSettings.FolderPath = selectedPath;
                                        }

                                        configService.SaveMediaSettings(currentMediaSettings);
                                        
                                        // Notify UI that settings have changed (this triggers SETTINGS_UPDATED_EVENT in JS)
                                        windowInstance.SendWebMessage("SETTINGS_UPDATED");
                                        // Also send specific signal for immediate UI update if desired
                                        windowInstance.SendWebMessage($"FOLDER_SELECTED:{selectedPath}");
                                   }
                                   catch (Exception ex)
                                   {
                                        Console.WriteLine($"Error saving folder selection: {ex.Message}");
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

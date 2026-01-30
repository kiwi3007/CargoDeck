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
using Microsoft.Extensions.Configuration;

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
        private static string? _logPath;

        public static void Log(string message)
        {
            var logLine = $"[{DateTime.Now:HH:mm:ss}] {message}";
            Console.WriteLine(logLine);
            if (_logPath != null)
            {
                try 
                {
                    // Log Rotation: Keep it under 10MB
                    var fileInfo = new FileInfo(_logPath);
                    if (fileInfo.Exists && fileInfo.Length > 10 * 1024 * 1024)
                    {
                        var oldLog = _logPath + ".old";
                        if (File.Exists(oldLog)) File.Delete(oldLog);
                        File.Move(_logPath, oldLog);
                    }
                    File.AppendAllText(_logPath, logLine + Environment.NewLine); 
                } 
                catch { }
            }
        }

        [STAThread]
        public static void Main(string[] args)
        {
            try 
            {
                var exePath = AppContext.BaseDirectory;
                
                // Temporary log path until config service is ready
                _logPath = Path.Combine(Path.GetTempPath(), "playerr_startup.log");
                Log("=== Playerr Startup Started ===");

                // LibUsb logic removed (Moved to Playerr.UsbHelper)

                var builder = WebApplication.CreateBuilder(new WebApplicationOptions
                {
                    Args = args,
                    ContentRootPath = exePath
                });
            // Add services
            System.Console.WriteLine("DEBUG: Registering Services...");
            builder.Services.AddControllers()
                .AddApplicationPart(typeof(Playerr.Api.V3.Games.GameController).Assembly)
                .AddNewtonsoftJson(options => {
                     options.SerializerSettings.ReferenceLoopHandling = Newtonsoft.Json.ReferenceLoopHandling.Ignore;
                     options.SerializerSettings.NullValueHandling = Newtonsoft.Json.NullValueHandling.Ignore;
                });

            // DEBUG: Print all discovered controllers to console
            var feature = new Microsoft.AspNetCore.Mvc.Controllers.ControllerFeature();
            builder.Services.AddMvc().PartManager.PopulateFeature(feature);
            Console.WriteLine($"[Startup-Debug] Discovered {feature.Controllers.Count} controllers:");
            foreach (var c in feature.Controllers)
            {
                Console.WriteLine($"[Startup-Debug] Controller: {c.Name} ({c.Namespace})");
            }
            
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
                         configPath = appDataConfig;
                         // exePath remains where the executable is
                     }
                }
            }
            
            // Note: ConfigurationService adds "/config" to the path passed to it
            var configService = new ConfigurationService(exePath);
            builder.Services.AddSingleton(configService);

            // Initialize Log Path
            _logPath = Path.Combine(configService.GetConfigDirectory(), "playerr.log");
            try { File.WriteAllText(_logPath, $"--- Playerr Startup {DateTime.Now} ---" + Environment.NewLine); } catch { }
            Log($"[Startup] EXE Path: {exePath}");
            Log($"[Startup] Config Path: {configService.GetConfigDirectory()}");

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

            // Switch USB
            builder.Services.AddSingleton<Playerr.Core.Switch.ISwitchUsbService, Playerr.Core.Switch.SwitchUsbService>();

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
            var isHeadless = args.Contains("--headless") || 
                             envVar == "true" || 
                             builder.Configuration.GetValue<bool>("HeadlessMode");
            
            Console.WriteLine($"[Startup] Checking Headless Mode: Args={string.Join(",", args)}, Config={builder.Configuration.GetValue<bool>("HeadlessMode")}, EnvVar={envVar}, Result={isHeadless}");

            if (!isHeadless)
            {
                // LOAD PERSISTENT SERVER SETTINGS
                var serverSettings = configService.LoadServerSettings();

                // Check for IP/Port overrides (Env Vars or Args)
                // Order: Args > Env Vars > Config > Defaults
                string? envIp = Environment.GetEnvironmentVariable("PLAYERR_IP");
                string? envPort = Environment.GetEnvironmentVariable("PLAYERR_PORT");
                
                string? argIp = null;
                string? argPort = null;
                
                for (int i = 0; i < args.Length; i++)
                {
                    if (args[i] == "--ip" && i + 1 < args.Length) argIp = args[i+1];
                    if (args[i] == "--port" && i + 1 < args.Length) argPort = args[i+1];
                }
                
                // Priority: Args > Env > Config > Default(127.0.0.1)
                string targetIpStr = argIp ?? envIp;
                if (targetIpStr == null)
                {
                     // Fallback to Config
                     targetIpStr = serverSettings.UseAllInterfaces ? "0.0.0.0" : "127.0.0.1";
                }
                
                System.Net.IPAddress targetIp = targetIpStr == "0.0.0.0" ? System.Net.IPAddress.Any : 
                                               (System.Net.IPAddress.TryParse(targetIpStr, out var ip) ? ip : System.Net.IPAddress.Loopback);

                // Priority: Args > Env > Config (if changed from default 5002) > Default (Null -> Auto)
                int? targetPort = null;
                if (!string.IsNullOrEmpty(argPort) && int.TryParse(argPort, out int p1)) targetPort = p1;
                else if (!string.IsNullOrEmpty(envPort) && int.TryParse(envPort, out int p2)) targetPort = p2;
                else if (serverSettings.Port != 5002) targetPort = serverSettings.Port; // Only override if user changed it from default

                builder.WebHost.ConfigureKestrel(serverOptions =>
                {
                    if (targetPort.HasValue)
                    {
                        // STRICT MODE: User specified a port. Bind only to it. Fail if busy.
                        serverOptions.Listen(targetIp, targetPort.Value);
                        Console.WriteLine($"[Startup] Bound to {targetIp}:{targetPort.Value} (Strict)");
                    }
                    else
                    {
                        // AUTO MODE (Default): Try preferred ports, fallback to dynamic
                        int[] preferredPorts = { 5002, 5003, 5004, 5005 };
                        bool bound = false;
                        
                        foreach (var port in preferredPorts)
                        {
                            try {
                                serverOptions.Listen(targetIp, port);
                                bound = true;
                                Console.WriteLine($"[Startup] Bound to {targetIp}:{port}");
                                break;
                            } catch { }
                        }
                        
                        if (!bound) {
                            serverOptions.Listen(targetIp, 0); // Total fallback
                            Console.WriteLine($"[Startup] Bound to {targetIp}:Dynamic (Fallback)");
                        }
                    }
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
                    Console.WriteLine("[Database] Checking for schema updates...");
                    try {
                        var connection = context.Database.GetDbConnection();
                        connection.Open();
                        using var cmdCheck = connection.CreateCommand();
                        
                        // Check ExecutablePath
                        cmdCheck.CommandText = "PRAGMA table_info(Games);";
                        var hasExePath = false;
                        var hasExternal = false;
                        using (var reader = cmdCheck.ExecuteReader())
                        {
                            while (reader.Read())
                            {
                                var colName = reader["name"].ToString();
                                if (colName == "ExecutablePath") hasExePath = true;
                                if (colName == "IsExternal") hasExternal = true;
                            }
                        }
                        
                        if (!hasExePath)
                        {
                            using var cmdAdd = connection.CreateCommand();
                            cmdAdd.CommandText = "ALTER TABLE Games ADD COLUMN ExecutablePath TEXT;";
                            cmdAdd.ExecuteNonQuery();
                            Console.WriteLine("[Schema] Added ExecutablePath column.");
                        }

                        if (!hasExternal)
                        {
                            using var cmdAdd = connection.CreateCommand();
                            cmdAdd.CommandText = "ALTER TABLE Games ADD COLUMN IsExternal INTEGER DEFAULT 0;";
                            cmdAdd.ExecuteNonQuery();
                            Console.WriteLine("[Schema] Added IsExternal column.");
                        }

                        connection.Close();
                    } catch (Exception ex) {
                        Console.WriteLine($"[Database] Schema check notice: {ex.Message}");
                    }

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
                        Console.WriteLine("[Database] Checking for schema updates (v0.4.0)...");
                        var connection = context.Database.GetDbConnection();
                        connection.Open();
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
                            "Genres TEXT",             // JSON (New)
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
                                cmd.ExecuteNonQuery();
                                Console.WriteLine($"[Database] Configured legacy column: {colDef.Split(' ')[0]}");
                            }
                            catch (Exception)
                            {
                                // Column likely exists. Ignore.
                            }
                        }
                        
                        connection.Close();
                    }
                    catch (Exception ex)
                    {
                        Console.WriteLine($"[Database] Schema check warning: {ex.Message}");
                    }

                    // [Schema Update] Ensure GameFiles table exists (v0.4.x)
                    try 
                    {
                        var connection = context.Database.GetDbConnection();
                        connection.Open();
                        using var cmdCheckTable = connection.CreateCommand();
                        cmdCheckTable.CommandText = "SELECT name FROM sqlite_master WHERE type='table' AND name='GameFiles';";
                        var tableName = cmdCheckTable.ExecuteScalar();

                        if (tableName == null)
                        {
                            Console.WriteLine("[Database] Creating missing GameFiles table...");
                            using var cmdCreateTable = connection.CreateCommand();
                            cmdCreateTable.CommandText = @"
                                CREATE TABLE GameFiles (
                                    Id INTEGER PRIMARY KEY AUTOINCREMENT,
                                    GameId INTEGER NOT NULL,
                                    RelativePath TEXT,
                                    Size INTEGER NOT NULL,
                                    DateAdded TEXT NOT NULL DEFAULT '0001-01-01 00:00:00',
                                    Quality TEXT,
                                    ReleaseGroup TEXT,
                                    Edition TEXT,
                                    Languages TEXT,
                                    CONSTRAINT FK_GameFiles_Games_GameId FOREIGN KEY (GameId) REFERENCES Games (Id) ON DELETE CASCADE
                                );
                                CREATE INDEX IX_GameFiles_GameId ON GameFiles (GameId);
                            ";
                            cmdCreateTable.ExecuteNonQuery();
                            Console.WriteLine("[Database] GameFiles table created.");
                        }
                        connection.Close();
                    }
                    catch (Exception ex)
                    {
                        Console.WriteLine($"[Database] GameFiles table check warning: {ex.Message}");
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

            app.Use(async (context, next) =>
            {
                Log($"[HTTP] {context.Request.Method} {context.Request.Path}");
                await next();
            });

            app.MapControllers();

            // Serve frontend - fallback to index.html for SPA routing
            if (Directory.Exists(uiPath))
            {
                app.MapFallback(context =>
                {
                    var indexPath = Path.Combine(uiPath, "index.html");
                    var html = File.ReadAllText(indexPath);
                    context.Response.ContentType = "text/html";
                    return context.Response.Body.WriteAsync(System.Text.Encoding.UTF8.GetBytes(html)).AsTask();
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
            Log("[Startup] Kestrel server started.");

            var server = app.Services.GetRequiredService<Microsoft.AspNetCore.Hosting.Server.IServer>();
            var addressFeature = server.Features.Get<Microsoft.AspNetCore.Hosting.Server.Features.IServerAddressesFeature>();

            // PROFESSIONAL: Get the assigned address and normalize to localhost
            string? rawAddress = addressFeature?.Addresses.FirstOrDefault();
            
            // Wait for address population if dynamic
            if (string.IsNullOrEmpty(rawAddress))
            {
                 Log("[Startup] Waiting for Kestrel address to be populated...");
                 for (int i = 0; i < 5 && string.IsNullOrEmpty(rawAddress); i++)
                 {
                     System.Threading.Thread.Sleep(100);
                     rawAddress = addressFeature?.Addresses.FirstOrDefault();
                 }
            }

            rawAddress ??= "http://localhost:5001"; // Fallback
            
            // Use 127.0.0.1 for internal alive-check
            string internalAddress = rawAddress;
            if (internalAddress.Contains("localhost")) internalAddress = internalAddress.Replace("localhost", "127.0.0.1");
            
            // Define the final UI address
            string address = rawAddress;
            if (address.Contains("127.0.0.1")) address = address.Replace("127.0.0.1", "localhost");
            if (address.Contains("[::1]")) address = address.Replace("[::1]", "localhost");
            if (address.Contains("0.0.0.0")) address = address.Replace("0.0.0.0", "localhost");

            // PRO-CHECK: Wait for the server to actually be ALIVE and serving content
            Log($"[Startup] Waiting for backend at {internalAddress}...");
            try 
            {
                using (var client = new System.Net.Http.HttpClient())
                {
                    client.Timeout = TimeSpan.FromSeconds(5);
                    for (int i = 0; i < 30; i++) // Try for up to 3 seconds
                    {
                        try {
                            var response = client.GetAsync(internalAddress).Result;
                            if (response.IsSuccessStatusCode) {
                                Log($"[Startup] Backend is ALIVE on {internalAddress}");
                                break;
                            }
                        } catch { }
                        System.Threading.Thread.Sleep(100);
                    }
                }
            }
            catch (Exception ex)
            {
                Log($"[Startup] Warning: Alive-check failed to execute: {ex.Message}");
            }

            Log($"[Startup] Playerr backend ready on: {address}");
            
            if (isHeadless)
            {
                 Log("Running in Headless Mode (Docker/Server). Press Ctrl+C to exit.");
                 app.WaitForShutdown();
            }
            else
            {
                // Launch Photino Window
                // This blocks until the window is closed
                try 
                {
                   // DEBUG: List all endpoints
                   var dataSources = app.Services.GetServices<Microsoft.AspNetCore.Routing.EndpointDataSource>();
                   foreach (var dataSource in dataSources)
                   {
                       foreach (var endpoint in dataSource.Endpoints)
                       {
                           if (endpoint is Microsoft.AspNetCore.Routing.RouteEndpoint routeEndpoint)
                           {
                               Log($"[Route] {routeEndpoint.RoutePattern.RawText} -> {routeEndpoint.DisplayName}");
                           }
                       }
                   }

                   Log("[UI] Initializing Photino Window...");
                   var window = new Photino.NET.PhotinoWindow()
                       .SetTitle("Playerr")
                       .SetUseOsDefaultSize(false)
                       .SetSize(new System.Drawing.Size(1280, 800))
                       .Center()
                       .SetResizable(true)
                       .SetDevToolsEnabled(true);
                   
                   bool isClosing = false;
                   window.WindowClosing += (s, e) => { 
                       isClosing = true; 
                       Log("[UI] Window is closing...");
                       return false; // Allow close
                   };
    
                   // Real-time library updates: Subscribe to scanner events
                   var scannerService = app.Services.GetRequiredService<MediaScannerService>();
                   
                   // Update library UI when a batch is finished
                    scannerService.OnBatchFinished += () => {
                        if (isClosing) return;
                        try {
                            window.Invoke(() => {
                                if (isClosing) return;
                                Log("[UI] Sending LIBRARY_UPDATED signal to frontend...");
                                try { window.SendWebMessage("LIBRARY_UPDATED"); } catch { }
                            });
                        } catch { }
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
                                         if (!isClosing)
                                         {
                                             windowInstance.Invoke(() => {
                                                 if (isClosing) return;
                                                 try {
                                                     windowInstance.SendWebMessage("SETTINGS_UPDATED");
                                                     windowInstance.SendWebMessage($"FOLDER_SELECTED:{selectedPath}");
                                                 } catch { }
                                             });
                                         }
                                   }
                                   catch (Exception ex)
                                   {
                                        Console.WriteLine($"Error saving folder selection: {ex.Message}");
                                   }
                               }
                           }
                       })
                        .Load(address); // Removed query string to avoid SPA routing issues
                       
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
            } // Close else
            } // Close try
            catch (Exception fatalEx)
            {
                Log($"[CRITICAL] Application failed to start: {fatalEx.Message}");
                Log(fatalEx.StackTrace ?? "No stack trace available.");
                // Ensure the console stays open if run manually
                Console.WriteLine("Press any key to exit...");
                try { Console.ReadKey(); } catch { }
            }
        }
    }
}

using Microsoft.AspNetCore.Mvc;
using System;
using System.IO;
using System.Linq;

namespace Playerr.Api.V3.Settings
{
    [ApiController]
    [Route("api/v3/[controller]")]
    public class ExploreController : ControllerBase
    {
        [HttpGet]
        public IActionResult ListFolders([FromQuery] string? path)
        {
            try
            {
                // Default to root if no path provided
                string targetPath = string.IsNullOrWhiteSpace(path) ? "/" : path;

                // For Windows compatibility during development, if path is empty or "/", use a default drive
                if (targetPath == "/" && System.Runtime.InteropServices.RuntimeInformation.IsOSPlatform(System.Runtime.InteropServices.OSPlatform.Windows))
                {
                    targetPath = "C:\\";
                }

                if (!Directory.Exists(targetPath))
                {
                    return BadRequest(new { error = "Directory does not exist" });
                }

                var entries = Directory.GetDirectories(targetPath)
                    .Select(d => new 
                    {
                        Name = Path.GetFileName(d),
                        Path = d.Replace("\\", "/"), // Normalize for web
                        IsDirectory = true
                    })
                    .OrderBy(d => d.Name)
                    .ToList();

                var parentPath = Path.GetDirectoryName(targetPath)?.Replace("\\", "/");

                return Ok(new
                {
                    CurrentPath = targetPath.Replace("\\", "/"),
                    ParentPath = parentPath,
                    Entries = entries
                });
            }
            catch (UnauthorizedAccessException)
            {
                return Forbid();
            }
            catch (Exception ex)
            {
                return BadRequest(new { error = ex.Message });
            }
        }

        [HttpGet("home")]
        public IActionResult GetHome()
        {
            // In Docker, /media or / is a good start. 
            // In Windows (dev), UserProfile or C:\
            string home = "/";
            
            if (System.Runtime.InteropServices.RuntimeInformation.IsOSPlatform(System.Runtime.InteropServices.OSPlatform.Windows))
            {
                home = Environment.GetFolderPath(Environment.SpecialFolder.UserProfile);
            }
            else if (Directory.Exists("/media"))
            {
                home = "/media";
            }

            return Ok(new { Path = home.Replace("\\", "/") });
        }
    }
}

using System;

namespace Playerr.Core.Configuration
{
    public class MediaSettings
    {
        public string FolderPath { get; set; } = string.Empty;
        public string Platform { get; set; } = "default";
        
        public bool IsConfigured => !string.IsNullOrWhiteSpace(FolderPath);
    }
}

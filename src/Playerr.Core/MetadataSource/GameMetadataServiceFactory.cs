using Playerr.Core.MetadataSource.Igdb;
using Playerr.Core.Configuration;
using Playerr.Core.MetadataSource.Steam;

namespace Playerr.Core.MetadataSource
{
    public interface IGameMetadataServiceFactory
    {
        GameMetadataService CreateService();
        void RefreshConfiguration();
    }

    public class GameMetadataServiceFactory : IGameMetadataServiceFactory
    {
        private readonly ConfigurationService _configService;
        private GameMetadataService? _currentService;

        public GameMetadataServiceFactory(ConfigurationService configService)
        {
            _configService = configService;
        }

        public GameMetadataService CreateService()
        {
            if (_currentService == null)
            {
                RefreshConfiguration();
            }
            return _currentService!;
        }

        public void RefreshConfiguration()
        {
            var igdbSettings = _configService.LoadIgdbSettings();
            var steamSettings = _configService.LoadSteamSettings();
            
            System.Console.WriteLine($"[MetadataFactory] Refreshing Configuration. IGDB Configured: {igdbSettings.IsConfigured}");

            // ALWAYS recreate the service to ensure fresh credentials are used
            if (igdbSettings.IsConfigured)
            {
                var igdbClient = new IgdbClient(igdbSettings.ClientId, igdbSettings.ClientSecret);
                var steamClient = new SteamClient(steamSettings.ApiKey);
                _currentService = new GameMetadataService(igdbClient, steamClient);
            }
            else
            {
                // Create a dummy client for when IGDB is not configured
                var dummyClient = new IgdbClient(string.Empty, string.Empty);
                var steamClient = new SteamClient(steamSettings.ApiKey);
                _currentService = new GameMetadataService(dummyClient, steamClient);
            }
        }
    }
}
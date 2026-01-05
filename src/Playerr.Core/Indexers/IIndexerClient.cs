using System.Collections.Generic;
using System.Threading.Tasks;
using Playerr.Core.Prowlarr; // For SearchResult

namespace Playerr.Core.Indexers
{
    public interface IIndexerClient
    {
        Task<List<SearchResult>> SearchAsync(string query, int[]? categories = null);
        Task<bool> TestConnectionAsync();
    }
}

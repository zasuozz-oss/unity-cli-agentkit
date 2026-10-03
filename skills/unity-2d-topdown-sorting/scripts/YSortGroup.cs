// YSortGroup.cs — copy into the game's runtime scripts (YSortGroup needs YSort); set the namespace and layer names.
using UnityEngine;
using UnityEngine.Rendering;

namespace Game
{
    /// <summary>
    /// Top-down front/back for a composite (a house, a landmark, a character made of parts):
    /// the SortingGroup's order follows the bottom of the art showing right now — active, enabled, non-WorldOverlay
    /// SpriteRenderers — so a site, a built house at any level and a broken landmark each sort by their own foot.
    /// Recomputed every LateUpdate: the parts flip from several views (state views,
    /// level sprite swaps) and a dirty hook in each costs more than ~250 cached min-scans per frame.
    /// Movers (hero) keep <see cref="YSort"/>; the map build tool adds this to every composite.
    /// </summary>
    [RequireComponent(typeof(SortingGroup))]
    public sealed class YSortGroup : MonoBehaviour
    {
        [Tooltip("Stable tiebreak against overlapping statics (set by the build tool's unique-order pass)")]
        [SerializeField] private int _offset;
        [Tooltip("Parts that never define the foot (e.g. a landmark's hint glow)")]
        [SerializeField] private SpriteRenderer[] _ignore = new SpriteRenderer[0];

        private SortingGroup _group;
        private SpriteRenderer[] _parts;
        private int _overlayId;

        public int Offset { get => _offset; set => _offset = value; }

        /// <summary>Lowest world y of the showing art; the root's y when nothing shows.</summary>
        public float Bottom()
        {
            if (_parts == null)
            {
                _parts = GetComponentsInChildren<SpriteRenderer>(true);
                _overlayId = SortingLayer.NameToID("WorldOverlay");
            }
            float foot = float.MaxValue;
            foreach (var sr in _parts)
            {
                if (sr == null || !sr.enabled || sr.sprite == null || !sr.gameObject.activeInHierarchy) continue;
                if (sr.sortingLayerID == _overlayId || System.Array.IndexOf(_ignore, sr) >= 0) continue;
                foot = Mathf.Min(foot, sr.bounds.min.y);
            }
            return foot == float.MaxValue ? transform.position.y : foot;
        }

        /// <summary>Re-reads the parts (call after adding/removing child renderers).</summary>
        public void Rescan() => _parts = null;

        public void Apply()
        {
            if (_group == null) _group = GetComponent<SortingGroup>();
            _group.sortingOrder = YSort.Order(Bottom()) + _offset;
        }

        private void LateUpdate() => Apply();
    }
}

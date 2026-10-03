// YSort.cs — copy into the game's runtime scripts (YSortGroup needs YSort); set the namespace and layer names.
using UnityEngine;

namespace Game
{
    /// <summary>
    /// Top-down front/back for things that move: sortingOrder from the feet (transform y) on the
    /// World sorting layer. Static map objects get the same order once, at build time
    /// (a build tool), so only movers carry this component.
    /// </summary>
    [RequireComponent(typeof(Renderer))]
    public sealed class YSort : MonoBehaviour
    {
        /// <summary>Orders per world unit (0.01 u steps). Keep |footY| × PerUnit inside ±32767 (the 16-bit order range); the
        /// World layer holds nothing else, so the range only has to fit there.</summary>
        public const float PerUnit = 100f;

        public static int Order(float footY) => Mathf.RoundToInt(-footY * PerUnit);

        [SerializeField] private int _offset = 1; // wins a tie against a prop whose foot is level with ours

        private Renderer _renderer;

        private void Awake()
        {
            _renderer = GetComponent<Renderer>();
            _renderer.sortingLayerName = "World";
        }

        private void LateUpdate() => _renderer.sortingOrder = Order(transform.position.y) + _offset;
    }
}
